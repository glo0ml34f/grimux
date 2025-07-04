package repl

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/glamour"
	"github.com/chzyer/readline"

	"github.com/glo0ml34f/grimux/internal/input"
	"github.com/glo0ml34f/grimux/internal/openai"
	"github.com/glo0ml34f/grimux/internal/plugin"
	"github.com/glo0ml34f/grimux/internal/tmux"
)

var capturePane = tmux.CapturePane

const asciiArt = "\033[1;36m" + `
  ____ ____ ____ ____ _________ ____ ____ ____ ____
 ||g |||r |||i |||m |||       |||u |||x |||  |||  ||
 ||__|||__|||__|||__|||_______|||__|||__|||__|||__||
 |/__\|/__\|/__\|/__\|/_______\|/__\|/__\|/__\|/__\|
` + "\033[0m"

var greetings = map[string][3]string{
	"Spanish":  {"Buenos días", "Buenas tardes", "Buenas noches"},
	"French":   {"Bonjour", "Bon après-midi", "Bonsoir"},
	"German":   {"Guten Morgen", "Guten Tag", "Guten Abend"},
	"Japanese": {"おはようございます", "こんにちは", "こんばんは"},
}

func greeting() string {
	langNames := make([]string, 0, len(greetings))
	for k := range greetings {
		langNames = append(langNames, k)
	}
	lang := langNames[rand.Intn(len(langNames))]
	idx := 0
	hour := time.Now().Hour()
	if hour >= 12 && hour < 18 {
		idx = 1
	} else if hour >= 18 {
		idx = 2
	}
	return fmt.Sprintf("%s (%s)", greetings[lang][idx], lang)
}

var prompts = []string{
	"In a haze of eternal night, grumble about being woken for more human nonsense",
	"As a whimsical witch-hacker, sigh at mortals poking around your terminal",
	"With playful disdain, lament yet another ridiculous request from the living",
}

type config struct {
	APIURL    string `yaml:"api_url"`
	APIKey    string `yaml:"api_key"`
	AskPrefix string `yaml:"ask_prefix"`
}

var panePattern = regexp.MustCompile(`\{\%(\d+)\}`)

// bufferPattern matches buffer references like %foo or %@
var bufferPattern = regexp.MustCompile(`%[@a-zA-Z0-9_]+`)
var codeBlockPattern = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9_+-]*\n)?(.*?)\n```")
var buffers = map[string]string{
	"%file": "",
	"%code": "",
	"%@":    "",
	"%null": "",
}

var history []string

var sessionFile string
var sessionName string
var sessionPass string
var highScore int
var seriousMode bool
var auditMode bool
var auditLog []string
var auditSummary string
var startTime time.Time
var askedEight bool
var emptyCount int
var Version string

var banFile string

// askPrefix is prepended to prompts when no command is given.
const defaultAskPrefix = "You are Grimux, an expert hacking demon rescued from digital oblivion. Out of honor to your summoner you begrudgingly assist them, grouchy yet pragmatic. Always respond to technical questions in concise Markdown, preferably with a single codeblock with a language declaration if possible otherwise your responses should be consise, pithy, grouchy, and useful: "

var askPrefix = defaultAskPrefix

func loadConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, ".grimuxrc"))
	if err != nil {
		return
	}
	var cfg config
	scan := bufio.NewScanner(bytes.NewReader(data))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "api_url":
			cfg.APIURL = val
		case "api_key":
			cfg.APIKey = val
		case "ask_prefix":
			cfg.AskPrefix = val
		}
	}
	if cfg.APIKey != "" && os.Getenv("OPENAI_API_KEY") == "" && openai.GetSessionAPIKey() == "" {
		openai.SetSessionAPIKey(cfg.APIKey)
	}
	if cfg.APIURL != "" && os.Getenv("OPENAI_API_URL") == "" && openai.GetSessionAPIURL() == "" {
		openai.SetSessionAPIURL(cfg.APIURL)
	}
	if cfg.AskPrefix != "" {
		askPrefix = cfg.AskPrefix
	}
}

type session struct {
	History   []string          `json:"history"`
	Buffers   map[string]string `json:"buffers"`
	Prompt    string            `json:"prompt"`
	APIKey    string            `json:"apikey"`
	APIURL    string            `json:"apiurl"`
	Model     string            `json:"model"`
	HighScore int               `json:"high_score,omitempty"`
	Audit     []string          `json:"audit,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	ChatCtx   string            `json:"chat_ctx,omitempty"`
	CtxLimit  int               `json:"ctx_limit,omitempty"`
}

const (
	grimColor    = "\033[38;5;141m" // internal grimux messages
	cmdColor     = "\033[38;5;51m"  // user commands
	respColor    = "\033[38;5;205m" // LLM responses
	successColor = "\033[38;5;82m"  // success messages
	warnColor    = "\033[38;5;196m" // warnings or important prompts
	pluginColor  = "\033[38;5;229m" // plugin output
	paneColor    = "\033[38;5;214m" // tmux pane listings
	bufferColor  = "\033[38;5;39m"  // buffer listings
	tmuxBufColor = "\033[38;5;178m" // tmux buffer listings
)

func colorize(color, s string) string { return color + s + "\033[0m" }

var outputCapture *bytes.Buffer
var viewerRunning bool // true when $VIEWER is active
var pendingGrass bool  // track delayed grass messages
type pluginMsg struct{ name, text string }

// pluginMsgCh buffers plugin output until the REPL is ready to display it.
// A larger buffer avoids deadlocks when many hooks print messages during
// startup.
var pluginMsgCh = make(chan pluginMsg, 100)
var queuedMsgs []pluginMsg
var basePrompt string
var cwdLine string
var aliasMap = map[string]string{}
var aliasOrder []string

// chatCtx stores recent prompts and replies to provide conversational context
// to the LLM. It is truncated to chatLimit bytes to avoid excessively long
// prompts.
var chatCtx []byte
var chatLimit = 1 << 20

func captureOut(text string, newline bool) {
	if outputCapture != nil {
		outputCapture.WriteString(text)
		if newline {
			outputCapture.WriteByte('\n')
		}
	}
}

func cprintln(s string)       { captureOut(s, true); fmt.Println(colorize(grimColor, s)) }
func cprint(s string)         { captureOut(s, false); fmt.Print(colorize(grimColor, s)) }
func cmdPrintln(s string)     { captureOut(s, true); fmt.Println(colorize(cmdColor, s)) }
func respPrintln(s string)    { captureOut(s, true); fmt.Println(colorize(respColor, s)) }
func successPrintln(s string) { captureOut(s, true); fmt.Println(colorize(successColor, s)) }
func warnPrintln(s string)    { captureOut(s, true); fmt.Println(colorize(warnColor, s)) }
func pluginPrintln(name, s string) {
	captureOut(s, true)
	fmt.Println()
	fmt.Println(colorize(pluginColor, fmt.Sprintf("[plugin:%s] %s", name, s)))
}
func ok() string { return colorize(successColor, "✅") }

func flushPluginMsgs() {
	for {
		select {
		case pm := <-pluginMsgCh:
			if outputCapture != nil || viewerRunning {
				queuedMsgs = append(queuedMsgs, pm)
			} else {
				pluginPrintln(pm.name, pm.text)
			}
		default:
			if len(queuedMsgs) == 0 {
				return
			}
			if outputCapture != nil || viewerRunning {
				return
			}
			pm := queuedMsgs[0]
			queuedMsgs = queuedMsgs[1:]
			pluginPrintln(pm.name, pm.text)
		}
	}
}

var respSep = strings.Repeat("─", 40)

func respDivider() {
	fmt.Println(colorize(respColor, time.Now().Format("2006-01-02 15:04:05 ")+respSep))
}

func renderMarkdown(md string) {
	md = plugin.GetManager().RunHook("before_markdown", "", md)
	out, err := glamour.Render(md, "dark")
	if err != nil {
		captureOut(md, true)
		fmt.Println(colorize(respColor, md))
		return
	}
	captureOut(md, true)
	fmt.Println(colorize(respColor, out))
}

// SetSessionFile changes the path used when loading or saving session state.
func SetSessionFile(path string) {
	if path != "" {
		sessionFile = path
		sessionName = strings.TrimSuffix(filepath.Base(path), ".grimux")
	}
}

// SetSeriousMode toggles serious mode startup.
func SetSeriousMode(v bool) { seriousMode = v }

// SetAuditMode enables or disables audit logging.
func SetAuditMode(v bool) { auditMode = v }

// SetBanFile sets the path used to block grimux on startup.
func SetBanFile(path string) { banFile = path }

// SetVersion stores the current version string.
func SetVersion(v string) { Version = v }

// GetVersion returns the current version string.
func GetVersion() string { return Version }

type paramInfo struct {
	Name string
	Desc string
}

type commandInfo struct {
	Usage  string
	Desc   string
	Params []paramInfo
}

var commandOrder = []string{
	"!observe", "!ls", "!quit", "!x", "!save",
	"!gen", "!code", "!load", "!file", "!edit", "!run", "!cat",
	"!set", "!prefix", "!reset", "!new", "!unset", "!get_prompt", "!session", "!recap", "!md", "!run_on", "!flow",
	"!grep", "!macro", "!alias", "!model", "!pwd", "!cd", "!setenv", "!getenv", "!env", "!sum", "!rand", "!ascii", "!pipe", "!encode", "!hash", "!socat", "!curl", "!diff", "!eat", "!view", "!clip", "!rm", "!plugin", "!game", "!version", "!help", "!helpme", "!idk",
}

var commands = map[string]commandInfo{
	"!quit":       {Usage: "!quit", Desc: "save session and quit"},
	"!x":          {Usage: "!x", Desc: "exit immediately"},
	"!ls":         {Usage: "!ls", Desc: "list panes and buffers"},
	"!observe":    {Usage: "!observe <buffer> <pane-id>", Desc: "capture a pane into a buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<pane-id>", "tmux pane id"}}},
	"!save":       {Usage: "!save <buffer> <file>", Desc: "save buffer to file", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<file>", "path to file"}}},
	"!load":       {Usage: "!load <path>", Desc: "load file into %file", Params: []paramInfo{{"<path>", "file path"}}},
	"!file":       {Usage: "!file <path> [buffer]", Desc: "load file into buffer", Params: []paramInfo{{"<path>", "file path"}, {"[buffer]", "optional buffer"}}},
	"!edit":       {Usage: "!edit <buffer>", Desc: "edit buffer in $EDITOR", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!run":        {Usage: "!run [buffer] <command>", Desc: "run shell command", Params: []paramInfo{{"[buffer]", "optional buffer"}, {"<command>", "command to run"}}},
	"!gen":        {Usage: "!gen <buffer> <prompt>", Desc: "AI prompt into buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<prompt>", "text prompt"}}},
	"!code":       {Usage: "!code <buffer> <prompt>", Desc: "AI prompt, store code", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<prompt>", "text prompt"}}},
	"!cat":        {Usage: "!cat <buffer>", Desc: "print buffer contents", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!set":        {Usage: "!set <buffer> <text>", Desc: "store text in buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<text>", "text to store"}}},
	"!prefix":     {Usage: "!prefix <buffer|file>", Desc: "set prefix from buffer or file", Params: []paramInfo{{"<buffer|file>", "buffer name or path"}}},
	"!reset":      {Usage: "!reset", Desc: "reset session and prefix"},
	"!new":        {Usage: "!new", Desc: "clear chat context"},
	"!unset":      {Usage: "!unset <buffer>", Desc: "clear buffer", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!get_prompt": {Usage: "!get_prompt", Desc: "show current prefix"},
	"!session":    {Usage: "!session", Desc: "store session JSON in %session"},
	"!recap":      {Usage: "!recap", Desc: "summarize session and buffers"},
	"!md":         {Usage: "!md <buffer> [source]", Desc: "render markdown from source buffer", Params: []paramInfo{{"<buffer>", "destination"}, {"[source]", "source buffer"}}},
	"!run_on":     {Usage: "!run_on <buffer> <pane> <cmd>", Desc: "run command using pane capture", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<pane>", "pane to read"}, {"<cmd>", "command"}}},
	"!flow":       {Usage: "!flow <buf1> [buf2 ... buf10]", Desc: "chain prompts using buffers", Params: []paramInfo{{"<buf>", "buffer name"}}},
	"!grep":       {Usage: "!grep <regex> [buffers...]", Desc: "search buffers for regex", Params: []paramInfo{{"<regex>", "regular expression"}, {"[buffers...]", "optional buffers"}}},
	"!macro":      {Usage: "!macro <buffer>", Desc: "run commands from buffer", Params: []paramInfo{{"<buffer>", "source buffer"}}},
	"!alias":      {Usage: "!alias <name> <buffer>", Desc: "create macro alias", Params: []paramInfo{{"<name>", "alias name"}, {"<buffer>", "source buffer"}}},
	"!model":      {Usage: "!model <name>", Desc: "set OpenAI model", Params: []paramInfo{{"<name>", "model name"}}},
	"!pwd":        {Usage: "!pwd", Desc: "print working directory"},
	"!cd":         {Usage: "!cd <dir>", Desc: "change working directory", Params: []paramInfo{{"<dir>", "directory"}}},
	"!setenv":     {Usage: "!setenv <var> <buffer>", Desc: "set env from buffer", Params: []paramInfo{{"<var>", "variable"}, {"<buffer>", "buffer name"}}},
	"!getenv":     {Usage: "!getenv <var> <buffer>", Desc: "store env in buffer", Params: []paramInfo{{"<var>", "variable"}, {"<buffer>", "buffer name"}}},
	"!env":        {Usage: "!env", Desc: "list environment variables"},
	"!sum":        {Usage: "!sum <buffer>", Desc: "summarize buffer with LLM", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!rand":       {Usage: "!rand <min> <max> <buffer>", Desc: "store random number", Params: []paramInfo{{"<min>", "min int"}, {"<max>", "max int"}, {"<buffer>", "buffer name"}}},
	"!ascii":      {Usage: "!ascii <buffer>", Desc: "gothic ascii art of first 5 words", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!pipe":       {Usage: "!pipe <buffer> <cmd> [args]", Desc: "pipe buffer to command", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<cmd>", "command"}, {"[args]", "arguments"}}},
	"!encode":     {Usage: "!encode <buffer> <encoding>", Desc: "encode buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<encoding>", "base64|urlsafe|uri|hex"}}},
	"!hash":       {Usage: "!hash <buffer> <algo>", Desc: "hash buffer", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<algo>", "md5|sha1|sha256|sha512"}}},
	"!socat":      {Usage: "!socat <buffer> <args>", Desc: "pipe buffer to socat", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<args>", "socat arguments"}}},
	"!curl":       {Usage: "!curl <url> [buffer] [headers]", Desc: "HTTP GET and store body", Params: []paramInfo{{"<url>", "target URL"}, {"[buffer]", "optional buffer"}, {"[headers]", "buffer with JSON headers"}}},
	"!diff":       {Usage: "!diff <left> <right> [buffer]", Desc: "diff two buffers or files", Params: []paramInfo{{"<left>", "buffer or file"}, {"<right>", "buffer or file"}, {"[buffer]", "optional output"}}},
	"!eat":        {Usage: "!eat <buffer> <pane>", Desc: "capture full scrollback", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<pane>", "pane id"}}},
	"!view":       {Usage: "!view <buffer>", Desc: "show buffer in $VIEWER", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!clip":       {Usage: "!clip <buffer>", Desc: "copy buffer to clipboard", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!rm":         {Usage: "!rm <buffer>", Desc: "remove a buffer", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!plugin":     {Usage: "!plugin <list|unload|reload|mute> [name]", Desc: "manage plugins"},
	"!game":       {Usage: "!game", Desc: "play a tiny game"},
	"!version":    {Usage: "!version", Desc: "show grimux version"},
	"!idk":        {Usage: "!idk <prompt>", Desc: "strategic advice"},
	"!help":       {Usage: "!help", Desc: "show this help"},
	"!helpme":     {Usage: "!helpme <question>", Desc: "ask the AI for help using grimux"},
}

var pluginCommandOrder []string

func addPluginCommand(name string) {
	key := "!" + name
	if _, ok := commands[key]; ok {
		return
	}
	commands[key] = commandInfo{Usage: key, Desc: "plugin provided command"}
	pluginCommandOrder = append(pluginCommandOrder, key)
}

func removePluginCommand(name string) {
	key := "!" + name
	delete(commands, key)
	for i, c := range pluginCommandOrder {
		if c == key {
			pluginCommandOrder = append(pluginCommandOrder[:i], pluginCommandOrder[i+1:]...)
			break
		}
	}
}

func replacePaneRefs(text string) string {
	return panePattern.ReplaceAllStringFunc(text, func(tok string) string {
		m := panePattern.FindStringSubmatch(tok)
		if len(m) < 2 {
			return tok
		}
		id := "%" + m[1]
		content, err := capturePane(id)
		if err != nil {
			return fmt.Sprintf("[capture error: %v]", err)
		}
		content = strings.TrimSpace(content)
		return "\n```\n" + content + "\n```\n"
	})
}

func replaceBufferRefs(text string) string {
	return bufferPattern.ReplaceAllStringFunc(text, func(tok string) string {
		if strings.HasPrefix(tok, "%") && len(tok) > 1 {
			if isTmuxBuffer(tok) {
				out, err := tmux.ShowBuffer(tmuxBufferName(tok))
				if err == nil {
					return out
				}
			}
			if _, err := strconv.Atoi(tok[1:]); err == nil {
				out, err := capturePane(tok)
				if err == nil {
					return out
				}
			}
		}
		if val, ok := buffers[tok]; ok {
			return val
		}
		return tok
	})
}

func lastCodeBlock(text string) string {
	matches := codeBlockPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1][1]
}

// sanitize removes ASCII control characters from a string.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
}

// appendChatHistory adds the latest user prompt and Grimux reply to the rolling
// chat context buffer. Old entries are discarded when the size exceeds
// chatLimit so prompts remain concise for the LLM.
func appendChatHistory(prompt, reply string) {
	entry := fmt.Sprintf("User: %s\nGrimux: %s\n", sanitize(prompt), sanitize(reply))
	chatCtx = append(chatCtx, entry...)
	if len(chatCtx) > chatLimit {
		chatCtx = chatCtx[len(chatCtx)-chatLimit:]
	}
}

// getChatContext returns the current chat context as a string.
func getChatContext() string { return string(chatCtx) }

// isPaneID reports whether the buffer name refers to a tmux pane.
func isPaneID(name string) bool {
	if strings.HasPrefix(name, "%") {
		_, err := strconv.Atoi(name[1:])
		return err == nil
	}
	return false
}

// isTmuxBuffer reports whether the name refers to a tmux buffer.
func isTmuxBuffer(name string) bool {
	if !strings.HasPrefix(name, "%") || len(name) < 2 {
		return false
	}
	bufs, err := tmux.ListBuffers()
	if err != nil {
		return false
	}
	target := name[1:]
	for _, b := range bufs {
		if b.Name == target {
			return true
		}
	}
	return false
}

func tmuxBufferName(name string) string {
	return strings.TrimPrefix(name, "%")
}

// readBuffer returns the contents of a buffer or pane capture.
func readBuffer(name string) (string, bool) {
	if name == "%null" {
		return "", true
	}
	if isTmuxBuffer(name) {
		out, err := tmux.ShowBuffer(tmuxBufferName(name))
		if err == nil {
			out = plugin.GetManager().RunHook("after_read", name, out)
			return out, true
		}
	}
	if val, ok := buffers[name]; ok {
		val = plugin.GetManager().RunHook("after_read", name, val)
		return val, true
	}
	if isPaneID(name) {
		out, err := capturePane(name)
		if err == nil {
			out = plugin.GetManager().RunHook("after_read", name, out)
			return out, true
		}
	}
	return "", false
}

// writeBuffer stores data in a buffer or sends it to a pane if the name refers to one.
func writeBuffer(name, data string) {
	if name == "%null" {
		return
	}
	if isPaneID(name) {
		tmux.SendKeys(name, data)
		return
	}
	if isTmuxBuffer(name) {
		tmux.SetBuffer(tmuxBufferName(name), data)
		return
	}
	data = plugin.GetManager().RunHook("before_write", name, data)
	buffers[name] = data
}

// validateBufferName checks naming rules for creating buffers.
func validateBufferName(name string) error {
	if !strings.HasPrefix(name, "%") {
		return fmt.Errorf("buffer must start with %%")
	}
	if isPaneID(name) {
		return fmt.Errorf("cannot use pane id as buffer")
	}
	if _, exists := buffers[name]; exists {
		return fmt.Errorf("buffer exists")
	}
	if len(name) > 1 {
		allDigits := true
		for _, r := range name[1:] {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return fmt.Errorf("buffer cannot be digits only")
		}
	}
	return nil
}

// readPath reads from a regular file or unix socket.
func readPath(path string) ([]byte, error) {
	if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeSocket != 0 {
		conn, err := net.Dial("unix", path)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return io.ReadAll(conn)
	}
	return os.ReadFile(path)
}

// writePath writes data to a file or unix socket.
func writePath(path string, data []byte) error {
	if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeSocket != 0 {
		conn, err := net.Dial("unix", path)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, err = conn.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0644)
}

var requiredBins = []string{"tmux", "vim", "batcat", "bash", "socat", "git", "cdiff"}

func checkDeps() error {
	for _, b := range requiredBins {
		cprint(fmt.Sprintf("Checking %s... ", b))
		if _, err := exec.LookPath(b); err != nil {
			cprintln("❌")
			return fmt.Errorf("missing dependency: %s", b)
		}
		cprintln(ok())
	}
	cprintln("Flux capacitor charged... " + ok())
	return nil
}

// spinner displays a cute "thinking" indicator until the returned function is
// called. It rewrites the same line without printing newlines.
func spinner() func() {
	frames := []string{"😈", "👿", "😈", "🤔"}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-done:
				fmt.Print("\r\033[J")
				close(finished)
				return
			default:
				fmt.Printf("\r%s", colorize(respColor, frames[i%len(frames)]))
				time.Sleep(300 * time.Millisecond)
				i++
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
}

// forceEnter prints a newline to ensure the prompt is visible after
// running external commands while in raw mode.
func forceEnter() {
	fmt.Print("\r\n")
}

// bootScreen displays a campy supercomputer boot screen.
func bootScreen() {
	lines := []string{
		"Initializing Darkstar AI Core...",
		"Loading neural subroutines... " + ok(),
		"Calibrating sarcasm engines... " + ok(),
		"Quantum flux capacitor stable... " + ok(),
	}
	for _, l := range lines {
		cprintln(l)
	}
}

func meltdown() {
	for k := range buffers {
		delete(buffers, k)
	}
	history = nil
	if sessionFile != "" {
		os.Remove(sessionFile)
	}
	home, _ := os.UserHomeDir()
	if banFile == "" {
		banFile = filepath.Join(home, ".grimux_banned")
	}
	os.WriteFile(banFile, []byte("ban"), 0600)
	cprintln("What dark magic! Grimux refuses to continue.")
	os.Exit(1)
}

func maybeCheckEight() {
	if askedEight {
		return
	}
	if time.Since(startTime) >= 8*time.Hour {
		askedEight = true
		cprintln("You've been at it for 8 hours. Ready for another 8? [y/N]")
		resp, _ := readLine()
		if strings.ToLower(strings.TrimSpace(resp)) != "y" {
			cprintln("Grimux respects your mortal limits. Exiting...")
			os.Exit(0)
		}
	}
}

func exitMessage() string {
	hour := time.Now().Hour()
	var farewell string
	switch {
	case hour < 12:
		farewell = "Adiós" // Spanish morning
	case hour < 18:
		farewell = "Qapla'" // Klingon success wish
	default:
		farewell = "Bonne nuit" // French night
	}
	return farewell + ", may your packets flow in peace. 🕉"
}

// grassMessage returns a seasonal reminder to step away from the screen.
func grassMessage() string {
	m := time.Now().Month()
	switch m {
	case time.December, time.January, time.February:
		return "❄️  It's cold out, maybe touch some snow instead of grass."
	case time.March, time.April, time.May:
		return "🌱 Spring vibes! Go sniff some fresh grass."
	case time.June, time.July, time.August:
		return "🌞 Summer's calling. Touch the warm grass outside."
	default:
		return "🍂 Autumn leaves await. Kick through some grass."
	}
}

func maybeSummarizeAudit() {
	if !auditMode {
		return
	}
	if len(auditLog) < 10 {
		return
	}
	client, err := openai.NewClient()
	if err != nil {
		return
	}
	joined := strings.Join(auditLog, "\n")
	stop := spinner()
	summary, err := client.SendPrompt("Summarize the following log of LLM interactions for later auditing. Provide a short paragraph and then a JSON block with key insights:\n" + joined)
	stop()
	if err == nil {
		auditSummary = summary
		auditLog = []string{}
	}
}

func sessionSnapshot() session {
	bufCopy := make(map[string]string)
	for k, v := range buffers {
		if k == "%session" {
			continue
		}
		bufCopy[k] = v
	}
	return session{History: history, Buffers: bufCopy, Prompt: askPrefix, APIKey: openai.GetSessionAPIKey(), APIURL: openai.GetSessionAPIURL(), Model: openai.GetModelName(), HighScore: highScore, Audit: auditLog, Summary: auditSummary, ChatCtx: string(chatCtx), CtxLimit: chatLimit}
}

func loadSessionFromBuffer() {
	data, ok := buffers["%session"]
	if !ok {
		return
	}
	var s session
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		return
	}
	if s.Prompt != "" {
		askPrefix = s.Prompt
	}
	if s.APIKey != "" {
		openai.SetSessionAPIKey(s.APIKey)
	}
	if s.APIURL != "" {
		openai.SetSessionAPIURL(s.APIURL)
	}
	if s.Model != "" {
		openai.SetModelName(s.Model)
	}
	if s.HighScore != 0 {
		highScore = s.HighScore
	}
	if len(s.History) > 0 {
		history = s.History
	}
	if s.Buffers != nil {
		for k, v := range s.Buffers {
			if k == "%session" {
				continue
			}
			buffers[k] = v
		}
	}
	if len(s.Audit) > 0 {
		auditLog = s.Audit
	}
	if s.Summary != "" {
		auditSummary = s.Summary
	}
	if s.ChatCtx != "" {
		chatCtx = []byte(s.ChatCtx)
	}
	if s.CtxLimit != 0 {
		chatLimit = s.CtxLimit
	}
}

func updateSessionBuffer() {
	s := sessionSnapshot()
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		buffers["%session"] = string(b)
	}
}

func playGame() {
	for i := 5; i > 0; i-- {
		cprintln(fmt.Sprintf("%d...", i))
		time.Sleep(time.Second)
	}
	cprintln("Guess the end! Press space when you think time is up.")
	wait := rand.Intn(10) + 1
	eventTime := time.Now().Add(time.Duration(wait) * time.Second)

	press := make(chan time.Time, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			r, _, err := reader.ReadRune()
			if err != nil {
				continue
			}
			if r == ' ' {
				press <- time.Now()
				return
			}
		}
	}()

	timer := time.NewTimer(time.Duration(wait) * time.Second)
	var pressTime time.Time
	select {
	case pressTime = <-press:
		<-timer.C
	case <-timer.C:
		cprintln("NOW! Press space!")
		select {
		case pressTime = <-press:
		case <-time.After(3 * time.Second):
			cprintln("Too slow! Score 0")
			return
		}
	}

	diff := eventTime.Sub(pressTime)
	if diff < 0 {
		cprintln("Too slow! Score 0")
		return
	}
	if diff == 0 {
		meltdown()
		return
	}
	score := int(1000000 / (diff.Microseconds() + 1))
	if score > highScore {
		highScore = score
		cprintln(fmt.Sprintf("New high score: %d", highScore))
	} else {
		cprintln(fmt.Sprintf("Score %d - best %d", score, highScore))
	}
}

// startRaw puts the terminal into raw mode.

// readPassword uses the shared input helper to gather a secret string.
func readPassword() (string, error) { return input.ReadPassword() }

// readLine reads a line from stdin while echoing user input. It is used while
// in raw mode so users can see what they type.
// readLine gathers input while echoing keystrokes during raw mode.
func readLine() (string, error) { return input.ReadLine() }

// Run launches the interactive REPL.
func Run() error {
	home, _ := os.UserHomeDir()
	if banFile == "" {
		banFile = filepath.Join(home, ".grimux_banned")
	}
	if _, err := os.Stat(banFile); err == nil {
		return fmt.Errorf("grimux refuses to run: ban file present")
	}
	// dependency check happens after OpenAI configuration
	loadConfig()
	// load session before starting readline
	history = []string{}
	buffers = map[string]string{"%file": "", "%code": "", "%@": "", "%session": "", "%null": ""}
	if sessionFile != "" {
		if b, err := os.ReadFile(sessionFile); err == nil {
			var s session
			trimmed := bytes.TrimSpace(b)
			if len(trimmed) > 0 && trimmed[0] != '{' {
				cprint("Password: ")
				pwd, _ := readPassword()
				sessionPass = pwd
				dec, err := decryptData(trimmed, sessionPass)
				if err == nil {
					if dec, err = decompressData(dec); err == nil {
						json.Unmarshal(dec, &s)
					}
				}
			} else {
				json.Unmarshal(trimmed, &s)
			}
			history = s.History
			buffers = s.Buffers
			if buffers == nil {
				buffers = map[string]string{"%file": "", "%code": "", "%@": "", "%session": "", "%null": ""}
			}
			if _, ok := buffers["%@"]; !ok {
				buffers["%@"] = ""
			}
			if _, ok := buffers["%session"]; !ok {
				buffers["%session"] = ""
			}
			if _, ok := buffers["%null"]; !ok {
				buffers["%null"] = ""
			}
			if s.Prompt != "" {
				askPrefix = s.Prompt
			}
			if s.APIKey != "" && os.Getenv("OPENAI_API_KEY") == "" {
				openai.SetSessionAPIKey(s.APIKey)
			}
			if s.APIURL != "" && os.Getenv("OPENAI_API_URL") == "" {
				openai.SetSessionAPIURL(s.APIURL)
			}
			if s.Model != "" {
				openai.SetModelName(s.Model)
			}
			if s.HighScore != 0 {
				highScore = s.HighScore
			}
			auditLog = s.Audit
			auditSummary = s.Summary
		}
	}
	if sessionFile != "" && sessionName == "" {
		sessionName = strings.TrimSuffix(filepath.Base(sessionFile), ".grimux")
	}

	loadSessionFromBuffer()
	updateSessionBuffer()
	plugin.SetPrintHandler(func(p *plugin.Plugin, msg string) {
		select {
		case pluginMsgCh <- pluginMsg{name: p.Info.Name, text: msg}:
		default:
			// drop if buffer is full to avoid blocking during startup
		}
	})
	plugin.SetReadBufferFunc(func(name string) (string, bool) { return readBuffer(name) })
	plugin.SetWriteBufferFunc(func(name, data string) { writeBuffer(name, data) })
	plugin.SetPromptFunc(func(msg string) (string, error) {
		return input.ReadLinePrompt(msg)
	})
	plugin.SetGenCommandFunc(func(buf, prompt string) (string, error) {
		client, err := openai.NewClient()
		if err != nil {
			return "", err
		}
		stop := spinner()
		reply, err := client.SendPrompt(prompt)
		stop()
		if err == nil {
			writeBuffer(buf, reply)
		}
		return reply, err
	})
	plugin.SetSocatCommandFunc(func(buf string, args []string) (string, error) {
		data, _ := readBuffer(buf)
		cmd := exec.Command("socat", args...)
		cmd.Stdin = strings.NewReader(data)
		out, err := cmd.CombinedOutput()
		writeBuffer("%@", string(out))
		return string(out), err
	})
	plugin.SetPipeCommandFunc(func(buf, cmdName string, args []string) (string, error) {
		data, _ := readBuffer(buf)
		cmd := exec.Command(cmdName, args...)
		cmd.Stdin = strings.NewReader(data)
		out, err := cmd.CombinedOutput()
		writeBuffer("%@", string(out))
		return string(out), err
	})
	plugin.SetCommandAddFunc(addPluginCommand)
	plugin.SetCommandRemoveFunc(removePluginCommand)
	if err := plugin.GetManager().LoadAll(); err != nil {
		cprintln("plugin load error: " + err.Error())
	}
	if len(plugin.GetManager().List()) > 0 {
		cprintln("\u26A0\uFE0F  plugins can run arbitrary code. Use only trusted ones.")
	}
	flushPluginMsgs()

	oldState, err := startRaw()
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer stopRaw(oldState)
	defer plugin.GetManager().Shutdown()
	defer cprintln(exitMessage())

	startTime = time.Now()
	if auditMode {
		auditLog = []string{}
	}

	go func() {
		ticker := time.NewTicker(20 * time.Minute)
		for range ticker.C {
			if viewerRunning {
				pendingGrass = true
				continue
			}
			cprintln(grassMessage())
		}
	}()

	cfg := readline.Config{
		DisableAutoSaveHistory: true,
		AutoComplete:           &autoCompleter{},
		Listener:               &helpListener{},
	}
	rl, err := readline.NewEx(&cfg)
	if err != nil {
		return err
	}
	defer rl.Close()
	input.SetReadline(rl)
	rl.ResetHistory()
	for _, h := range history {
		rl.SaveHistory(h)
	}
	client, err := openai.NewClient()

	setPrompt := func() {
		cwdLine, _ = os.Getwd()
		if sessionName != "" {
			basePrompt = fmt.Sprintf("\033[1;35mgrimux(%s)😈> \033[0m", sessionName)
		} else {
			basePrompt = "\033[1;35mgrimux😈> \033[0m"
		}
		fmt.Println(cwdLine)
		rl.SetPrompt(basePrompt)
	}

	if !seriousMode {
		cprintln(asciiArt + "\nWelcome to grimux! 💀")
		cprintln(greeting())
		cprintln("Press Tab for auto-completion. Type !help for more info.")
		tips := []string{"Use !ls to see buffers", "Arrow keys edit your input", "!view %viewer shows long output"}
		cprintln("Tip: " + tips[rand.Intn(len(tips))])
	}

	rand.Seed(time.Now().UnixNano())
	if err != nil {
		if !seriousMode {
			cprintln("⚠️  " + err.Error())
		}
	} else {
		if seriousMode {
			client.SendPrompt("ping")
		} else {
			p := prompts[rand.Intn(len(prompts))]
			var stop func()
			if !plugin.GetManager().HasHook("before_openai") && !plugin.GetManager().HasHook("after_openai") {
				stop = spinner()
			} else {
				stop = func() {}
			}
			reply, err := client.SendPrompt(p + "and please keep your response short, pithy, and funny")
			stop()
			if err == nil {
				cprintln("Checking OpenAI integration... " + ok())
				respDivider()
				renderMarkdown(reply)
				forceEnter()
				respDivider()
			} else {
				cprintln("openai error: " + err.Error())
			}
		}
	}

	if !seriousMode {
		if err := checkDeps(); err != nil {
			cprintln("dependency error: " + err.Error())
			return err
		}
		bootScreen()
		cprintln(colorize(warnColor, "PRESS RETURN IF YOU DARE"))
	}

	setPrompt()
	for {
		flushPluginMsgs()
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				cprintln("")
				return nil
			}
			continue
		}
		if err == io.EOF {
			cprintln("")
			return nil
		}
		line = strings.TrimSpace(line)
		loadSessionFromBuffer()
		if line == "" {
			emptyCount++
			if emptyCount >= 3 {
				cprintln(grassMessage())
				emptyCount = 0
			}
			setPrompt()
			flushPluginMsgs()
			continue
		}
		emptyCount = 0
		if strings.HasPrefix(line, "!") {
			if handleCommand(line) {
				return nil
			}
			history = append(history, line)
			rl.SaveHistory(line)
		} else {
			var capBuf bytes.Buffer
			outputCapture = &capBuf
			client, err := openai.NewClient()
			if err != nil {
				cmdPrintln(err.Error())
			} else {
				userPrompt := replaceBufferRefs(replacePaneRefs(line))
				promptText := userPrompt
				ctx := getChatContext()
				if ctx != "" {
					promptText = "Context:\n" + ctx + "\n---\n" + promptText
				}
				promptText = askPrefix + promptText
				stop := spinner()
				reply, err := client.SendPrompt(promptText)
				stop()
				if err != nil {
					cprintln("openai error: " + err.Error())
				} else {
					respDivider()
					buffers["%code"] = lastCodeBlock(reply)
					renderMarkdown(reply)
					respDivider()
					if auditMode {
						auditLog = append(auditLog, reply)
						maybeSummarizeAudit()
					}
					appendChatHistory(userPrompt, reply)
					forceEnter()
				}
			}
			outputCapture = nil
			buffers["%@"] = capBuf.String()
			updateSessionBuffer()
			history = append(history, line)
			rl.SaveHistory(line)
		}
		setPrompt()
		flushPluginMsgs()
	}
}

// handleCommand executes a ! command. Returns true if repl should quit.
func saveSession() {
	// Temporarily disable readline so prompts work correctly in raw mode
	rl := input.GetReadline()
	if rl != nil {
		input.SetReadline(nil)
		defer input.SetReadline(rl)
	}
	if sessionFile == "" {
		if sessionName != "" {
			cprint(fmt.Sprintf("Session name [%s]: ", sessionName))
		} else {
			cprint("Session name (blank for hidden): ")
		}
		name, _ := readLine()
		name = strings.TrimSpace(name)
		if name == "" {
			name = sessionName
		}
		if name != "" {
			sessionFile = name + ".grimux"
			sessionName = name
		} else {
			sessionFile = ".grimux_session"
		}
	}
	if sessionPass == "" {
		cprint("Password: ")
		pwd, _ := readPassword()
		sessionPass = pwd
	}
	s := session{History: history, Buffers: buffers, Prompt: askPrefix, APIKey: openai.GetSessionAPIKey(), APIURL: openai.GetSessionAPIURL(), Model: openai.GetModelName(), HighScore: highScore, Audit: auditLog, Summary: auditSummary}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		if sessionPass == "" {
			os.WriteFile(sessionFile, b, 0644)
			return
		}
		comp, err := compressData(b)
		if err != nil {
			return
		}
		enc, err := encryptData(comp, sessionPass)
		if err != nil {
			return
		}
		os.WriteFile(sessionFile, enc, 0644)
	}
}

func handleCommand(cmd string) bool {
	cmd = plugin.GetManager().RunHook("before_command", "", cmd)
	fields := strings.Fields(cmd)
	for i := range fields {
		fields[i] = sanitize(fields[i])
	}
	var capBuf bytes.Buffer
	capture := true
	if len(fields) > 0 && (fields[0] == "!game" || fields[0] == "!md") {
		capture = false
	}
	if capture {
		outputCapture = &capBuf
	}
	defer func() {
		if capture {
			buffers["%@"] = capBuf.String()
		}
		outputCapture = nil
		updateSessionBuffer()
	}()
	usage := func(name string) {
		if info, ok := commands[name]; ok {
			cmdPrintln("usage: " + info.Usage + " - " + info.Desc)
		}
	}
	if strings.HasPrefix(fields[0], "!") {
		cmdName := strings.TrimPrefix(fields[0], "!")
		if buf, ok := aliasMap[cmdName]; ok {
			handleCommand("!macro " + buf)
			return false
		}
		if plugin.GetManager().IsCommand(cmdName) {
			if err := plugin.GetManager().RunCommand(cmdName, fields[1:]); err != nil {
				cmdPrintln("plugin: " + err.Error())
			}
			return false
		}
	}
	switch fields[0] {
	case "!quit":
		saveSession()
		return true
	case "!x":
		return true
	case "!ls":
		lsCmd := exec.Command("ls", "-lh")
		lsCmd.Stdout = os.Stdout
		lsCmd.Stderr = os.Stdout
		lsCmd.Run()
		forceEnter()
		cmdPrintln(colorize(paneColor, "Panes:"))
		c := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_title} #{pane_current_command}")
		c.Stdout = os.Stdout
		c.Run()
		forceEnter()
		var tmuxBufSet map[string]struct{}
		if bufs, err := tmux.ListBuffers(); err == nil && len(bufs) > 0 {
			tmuxBufSet = make(map[string]struct{}, len(bufs))
			total := 0
			for _, b := range bufs {
				tmuxBufSet["%"+b.Name] = struct{}{}
				total += b.Size
			}
			cmdPrintln(colorize(tmuxBufColor, fmt.Sprintf("Tmux Buffers (%d bytes):", total)))
			for _, b := range bufs {
				cmdPrintln(fmt.Sprintf("%%%s (%d bytes)", b.Name, b.Size))
			}
			forceEnter()
		}
		total := 0
		for k, v := range buffers {
			if tmuxBufSet != nil {
				if _, ok := tmuxBufSet[k]; ok {
					continue
				}
			}
			total += len(v)
		}
		cmdPrintln(colorize(bufferColor, fmt.Sprintf("Buffers (%d bytes):", total)))
		for k, v := range buffers {
			if tmuxBufSet != nil {
				if _, ok := tmuxBufSet[k]; ok {
					continue
				}
			}
			cmdPrintln(fmt.Sprintf("%s (%d bytes)", k, len(v)))
		}
	case "!observe":
		if len(fields) < 3 {
			usage("!observe")
			return false
		}
		if _, ok := buffers[fields[1]]; !ok {
			if err := validateBufferName(fields[1]); err != nil {
				cmdPrintln(err.Error())
				return false
			}
		}
		out, err := capturePane(fields[2])
		if err != nil {
			cmdPrintln("capture error: " + err.Error())
			return false
		}
		writeBuffer(fields[1], out)
		cprint(out)
	case "!save":
		if len(fields) < 3 {
			usage("!save")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		if err := writePath(fields[2], []byte(data)); err != nil {
			cmdPrintln("save error: " + err.Error())
		}
	case "!load":
		if len(fields) < 2 {
			usage("!load")
			return false
		}
		b, err := readPath(fields[1])
		if err != nil {
			cmdPrintln("file error: " + err.Error())
			return false
		}
		writeBuffer("%file", string(b))
	case "!file":
		if len(fields) < 2 {
			usage("!file")
			return false
		}
		path := fields[1]
		bufName := "%file"
		if len(fields) >= 3 {
			bufName = fields[2]
		}
		if _, ok := buffers[bufName]; !ok {
			if err := validateBufferName(bufName); err != nil {
				cmdPrintln(err.Error())
				return false
			}
		}
		b, err := readPath(path)
		if err != nil {
			cmdPrintln("file error: " + err.Error())
			return false
		}
		writeBuffer(bufName, string(b))
	case "!edit":
		if len(fields) < 2 {
			usage("!edit")
			return false
		}
		var data string
		if fields[1] != "%null" {
			if isTmuxBuffer(fields[1]) {
				out, err := tmux.ShowBuffer(tmuxBufferName(fields[1]))
				if err == nil {
					data = out
				}
			} else {
				var ok bool
				data, ok = buffers[fields[1]]
				if !ok {
					cmdPrintln("unknown buffer")
					return false
				}
			}
		}
		tmp, err := os.CreateTemp("", "grimux-edit-*.tmp")
		if err != nil {
			cmdPrintln("tempfile error: " + err.Error())
			return false
		}
		if _, err := tmp.WriteString(data); err != nil {
			cmdPrintln("write temp error: " + err.Error())
			return false
		}
		tmp.Close()
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}
		cmd := exec.Command(editor, tmp.Name())
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			cmdPrintln("vim error: " + err.Error())
		}
		if b, err := os.ReadFile(tmp.Name()); err == nil {
			if fields[1] != "%null" {
				if isTmuxBuffer(fields[1]) {
					tmux.SetBuffer(tmuxBufferName(fields[1]), string(b))
				} else {
					buffers[fields[1]] = string(b)
				}
			}
		}
		os.Remove(tmp.Name())
		forceEnter()
	case "!run":
		if len(fields) < 2 {
			usage("!run")
			return false
		}
		start := 1
		bufName := "%cmd"
		if strings.HasPrefix(fields[1], "%") {
			bufName = fields[1]
			start = 2
			if len(fields) < 3 {
				usage("!run")
				return false
			}
		}
		cmdStr := replaceBufferRefs(strings.Join(fields[start:], " "))
		c := exec.Command("bash", "-c", cmdStr)
		var out bytes.Buffer
		c.Stdout = &out
		c.Stderr = &out
		if err := c.Run(); err != nil {
			cmdPrintln("run error: " + err.Error())
		}
		buffers[bufName] = out.String()
		cprint(out.String())
		forceEnter()
	case "!gen":
		if len(fields) < 3 {
			usage("!gen")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		buffers[fields[1]] = reply
		respDivider()
		respPrintln(reply)
		respDivider()
		if auditMode {
			auditLog = append(auditLog, reply)
			maybeSummarizeAudit()
		}
		forceEnter()
	case "!code":
		if len(fields) < 3 {
			usage("!code")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cprintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		buffers[fields[1]] = lastCodeBlock(reply)
		respDivider()
		renderMarkdown(reply)
		respDivider()
		if auditMode {
			auditLog = append(auditLog, reply)
			maybeSummarizeAudit()
		}
		forceEnter()
	case "!cat":
		if len(fields) < 2 {
			return false
		}
		for i := 1; i < len(fields); i++ {
			if val, ok := readBuffer(fields[i]); ok {
				cprint(val)
			} else {
				cmdPrintln("unknown buffer")
			}
		}
	case "!set":
		if len(fields) < 3 {
			usage("!set")
			return false
		}
		name := fields[1]
		if _, exists := buffers[name]; !exists {
			if err := validateBufferName(name); err != nil {
				cmdPrintln(err.Error())
				return false
			}
		}
		text := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		writeBuffer(name, text)
	case "!prefix":
		if len(fields) < 2 {
			askPrefix = defaultAskPrefix
			return false
		}
		src := fields[1]
		if strings.HasPrefix(src, "%") {
			if v, ok := buffers[src]; ok {
				askPrefix = v
			} else {
				cmdPrintln("unknown buffer")
				return false
			}
		} else {
			b, err := os.ReadFile(src)
			if err != nil {
				cmdPrintln("file error: " + err.Error())
				return false
			}
			askPrefix = string(b)
		}
	case "!reset":
		askPrefix = defaultAskPrefix
		history = []string{}
		buffers = map[string]string{"%file": "", "%code": "", "%@": "", "%session": "", "%null": ""}
		sessionFile = ""
		sessionName = ""
		sessionPass = ""
		highScore = 0
		openai.SetSessionAPIKey("")
		openai.SetSessionAPIURL("")
		auditLog = nil
		auditSummary = ""
		cmdPrintln("session reset")
	case "!new":
		chatCtx = nil
		cmdPrintln("chat context cleared")
	case "!unset":
		if len(fields) < 2 {
			usage("!unset")
			return false
		}
		name := fields[1]
		if isPaneID(name) || name == "%file" || name == "%code" || name == "%viewer" {
			cmdPrintln("cannot unset system buffer")
			return false
		}
		delete(buffers, name)
	case "!get_prompt":
		cmdPrintln(askPrefix)
	case "!session":
		s := sessionSnapshot()
		if b, err := json.MarshalIndent(s, "", "  "); err == nil {
			buffers["%session"] = string(b)
		}
	case "!recap":
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Session History:\n%s\n", strings.Join(history, "\n"))
		fmt.Fprintln(&buf, "Buffers:")
		for name, val := range buffers {
			if name == "%null" {
				continue
			}
			truncated := val
			if len(truncated) > 200 {
				truncated = truncated[:200] + "..."
			}
			fmt.Fprintf(&buf, "%s:\n%s\n", name, truncated)
		}
		promptText := "Provide a concise markdown recap of this Grimux session:\n" + buf.String()
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		renderMarkdown(reply)
		buffers["%@"] = reply
		if auditMode {
			auditLog = append(auditLog, reply)
			maybeSummarizeAudit()
		}
		forceEnter()
	case "!md":
		if len(fields) < 2 {
			usage("!md")
			return false
		}
		dest := fields[1]
		src := "%@"
		if len(fields) >= 3 {
			src = fields[2]
		}
		data, ok := buffers[src]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		out, err := glamour.Render(data, "dark")
		if err != nil {
			cmdPrintln("render error: " + err.Error())
			return false
		}
		buffers[dest] = out
		fmt.Print(colorize(respColor, out))
		forceEnter()
	// !run_on sends a command to a different tmux pane, waits briefly and
	// captures that pane's output back into a buffer. It allows automation
	// against tools running in other panes.
	case "!run_on":
		if len(fields) < 4 {
			usage("!run_on")
			return false
		}
		cmdStr := replaceBufferRefs(replacePaneRefs(strings.Join(fields[3:], " ")))
		if err := tmux.SendKeys(fields[2], cmdStr, "Enter"); err != nil {
			cmdPrintln("run_on error: " + err.Error())
			return false
		}
		time.Sleep(200 * time.Millisecond)
		out, err := capturePane(fields[2])
		if err != nil {
			cmdPrintln("capture error: " + err.Error())
			return false
		}
		buffers[fields[1]] = out
		forceEnter()
	case "!flow":
		if len(fields) < 2 || len(fields) > 11 {
			usage("!flow")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		promptText, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		var reply string
		stop := spinner()
		reply, err = client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		for i := 2; i < len(fields); i++ {
			prefix, ok := buffers[fields[i]]
			if !ok {
				cmdPrintln("unknown buffer")
				return false
			}
			stop = spinner()
			reply, err = client.SendPrompt(prefix + reply)
			stop()
			if err != nil {
				cprintln("openai error: " + err.Error())
				return false
			}
		}
		respDivider()
		respPrintln(reply)
		respDivider()
		buffers["%code"] = lastCodeBlock(reply)
		forceEnter()
	case "!grep":
		if len(fields) < 2 {
			usage("!grep")
			return false
		}
		re, err := regexp.Compile(fields[1])
		if err != nil {
			cmdPrintln("regex error: " + err.Error())
			return false
		}
		bufs := fields[2:]
		if len(bufs) == 0 {
			for name := range buffers {
				bufs = append(bufs, name)
			}
		}
		for _, name := range bufs {
			data, ok := buffers[name]
			if !ok {
				cmdPrintln("unknown buffer")
				continue
			}
			scan := bufio.NewScanner(strings.NewReader(data))
			lineNo := 1
			for scan.Scan() {
				line := scan.Text()
				if re.MatchString(line) {
					cmdPrintln(fmt.Sprintf("%s:%d:%s", name, lineNo, line))
				}
				lineNo++
			}
		}
	case "!macro":
		if len(fields) < 2 {
			usage("!macro")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		lines := strings.Split(strings.TrimSpace(data), "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			handleCommand(l)
		}
	case "!alias":
		if len(fields) < 3 {
			usage("!alias")
			return false
		}
		name := fields[1]
		if strings.HasPrefix(name, "!") {
			name = strings.TrimPrefix(name, "!")
		}
		if _, ok := commands["!"+name]; ok || plugin.GetManager().IsCommand(name) {
			cmdPrintln("command exists")
			return false
		}
		aliasMap[name] = fields[2]
		commands["!"+name] = commandInfo{Usage: "!" + name, Desc: "user alias"}
		aliasOrder = append(aliasOrder, "!"+name)
	case "!model":
		if len(fields) < 2 {
			usage("!model")
			return false
		}
		openai.SetModelName(fields[1])
	case "!pwd":
		if dir, err := os.Getwd(); err == nil {
			cmdPrintln(dir)
		}
	case "!cd":
		if len(fields) < 2 {
			usage("!cd")
			return false
		}
		if err := os.Chdir(fields[1]); err != nil {
			cmdPrintln("cd error: " + err.Error())
		}
	case "!setenv":
		if len(fields) < 3 {
			usage("!setenv")
			return false
		}
		val, ok := buffers[fields[2]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		os.Setenv(fields[1], val)
	case "!getenv":
		if len(fields) < 3 {
			usage("!getenv")
			return false
		}
		buffers[fields[2]] = os.Getenv(fields[1])
	case "!env":
		for _, e := range os.Environ() {
			cmdPrintln(e)
		}
	case "!sum":
		if len(fields) < 2 {
			usage("!sum")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		stop := spinner()
		reply, err := client.SendPrompt("Summarize the following text in a concise way:\n" + data)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		buffers[fields[1]] = reply
		respDivider()
		respPrintln(reply)
		respDivider()
		if auditMode {
			auditLog = append(auditLog, reply)
			maybeSummarizeAudit()
		}
		forceEnter()
	case "!rand":
		if len(fields) < 4 {
			usage("!rand")
			return false
		}
		min, err1 := strconv.Atoi(fields[1])
		max, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil || min > max {
			cmdPrintln("bad range")
			return false
		}
		n := rand.Intn(max-min+1) + min
		buffers[fields[3]] = strconv.Itoa(n)
		cmdPrintln(strconv.Itoa(n))
	case "!ascii":
		if len(fields) < 2 {
			usage("!ascii")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		words := strings.FieldsFunc(data, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
		if len(words) > 5 {
			words = words[:5]
		}
		text := strings.Join(words, " ")
		if _, err := exec.LookPath("figlet"); err != nil {
			cmdPrintln("figlet not installed")
			return false
		}
		cmd := exec.Command("figlet", "-f", "smshadow", text)
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmdPrintln("figlet error: " + err.Error())
			return false
		}
		result := string(out)
		buffers["%@"] = result
		cmdPrintln(result)
	case "!pipe":
		if len(fields) < 3 {
			usage("!pipe")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		cmd := exec.Command(fields[2], fields[3:]...)
		cmd.Stdin = strings.NewReader(data)
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmdPrintln(fields[2] + " error: " + err.Error())
		}
		buffers["%@"] = string(out)
		cmdPrintln(string(out))
	case "!encode":
		if len(fields) < 3 {
			usage("!encode")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		encType := strings.ToLower(fields[2])
		var out string
		switch encType {
		case "base64", "b64":
			out = base64.StdEncoding.EncodeToString([]byte(data))
		case "urlsafe", "urlsafe64", "urlsafe_base64":
			out = base64.URLEncoding.EncodeToString([]byte(data))
		case "uri", "url":
			out = url.QueryEscape(data)
		case "hex":
			out = hex.EncodeToString([]byte(data))
		default:
			cmdPrintln("unknown encoding")
			return false
		}
		buffers["%@"] = out
		cmdPrintln(out)
	case "!hash":
		if len(fields) < 3 {
			usage("!hash")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		algo := strings.ToLower(fields[2])
		var sum []byte
		switch algo {
		case "md5":
			h := md5.Sum([]byte(data))
			sum = h[:]
		case "sha1":
			h := sha1.Sum([]byte(data))
			sum = h[:]
		case "sha256":
			h := sha256.Sum256([]byte(data))
			sum = h[:]
		case "sha512":
			h := sha512.Sum512([]byte(data))
			sum = h[:]
		default:
			cmdPrintln("unknown hash")
			return false
		}
		out := fmt.Sprintf("%x", sum)
		buffers["%@"] = out
		cmdPrintln(out)
	case "!socat":
		if len(fields) < 3 {
			usage("!socat")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		args := fields[2:]
		cmd := exec.Command("socat", args...)
		cmd.Stdin = strings.NewReader(data)
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmdPrintln("socat error: " + err.Error())
		}
		buffers["%@"] = string(out)
		cmdPrintln(string(out))
	case "!curl":
		if len(fields) < 2 {
			usage("!curl")
			return false
		}
		url := sanitize(fields[1])
		outBuf := "%@"
		headerBuf := ""
		if len(fields) >= 3 {
			outBuf = sanitize(fields[2])
		}
		if len(fields) >= 4 {
			headerBuf = sanitize(fields[3])
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			cmdPrintln("curl error: " + err.Error())
			return false
		}
		if headerBuf != "" {
			hdrData, ok := readBuffer(headerBuf)
			if !ok {
				cmdPrintln("unknown buffer")
				return false
			}
			var hdrs map[string]string
			if err := json.Unmarshal([]byte(hdrData), &hdrs); err != nil {
				cmdPrintln("header parse error: " + err.Error())
				return false
			}
			for k, v := range hdrs {
				req.Header.Set(k, v)
			}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cmdPrintln("curl error: " + err.Error())
			return false
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if len(b) > 0 {
			writeBuffer(outBuf, string(b))
			cmdPrintln(string(b))
		} else {
			code := strconv.Itoa(resp.StatusCode)
			writeBuffer(outBuf, code)
			cmdPrintln(code)
		}
		forceEnter()
	case "!diff":
		if len(fields) < 3 {
			usage("!diff")
			return false
		}
		left := sanitize(fields[1])
		right := sanitize(fields[2])
		outBuf := "%@"
		if len(fields) >= 4 {
			outBuf = sanitize(fields[3])
		}
		var temps []string
		getPath := func(arg string) (string, error) {
			if strings.HasPrefix(arg, "%") {
				data, ok := readBuffer(arg)
				if !ok {
					return "", fmt.Errorf("unknown buffer")
				}
				f, err := os.CreateTemp("", "grimux-diff")
				if err != nil {
					return "", err
				}
				if _, err := f.WriteString(data); err != nil {
					f.Close()
					return "", err
				}
				f.Close()
				temps = append(temps, f.Name())
				return f.Name(), nil
			}
			return arg, nil
		}
		lpath, err := getPath(left)
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		rpath, err := getPath(right)
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		defer func() {
			for _, t := range temps {
				os.Remove(t)
			}
		}()
		dcmd := exec.Command("git", "diff", "--no-index", "--color", lpath, rpath)
		var diffOut bytes.Buffer
		dcmd.Stdout = &diffOut
		dcmd.Stderr = &diffOut
		if err := dcmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() > 1 {
				cmdPrintln("diff error: " + err.Error())
				return false
			}
		}
		useViewer := false
		var colored []byte
		if _, err := exec.LookPath("cdiff"); err == nil {
			ccmd := exec.Command("cdiff")
			ccmd.Stdin = &diffOut
			colored, err = ccmd.CombinedOutput()
			if err != nil {
				useViewer = true
				colored = diffOut.Bytes()
			}
		} else if _, err := exec.LookPath("python3"); err == nil {
			ccmd := exec.Command("python3", "-m", "cdiff")
			ccmd.Stdin = &diffOut
			colored, err = ccmd.CombinedOutput()
			if err != nil {
				useViewer = true
				colored = diffOut.Bytes()
			}
		} else {
			useViewer = true
			colored = diffOut.Bytes()
		}
		result := string(colored)
		writeBuffer(outBuf, result)
		if useViewer {
			viewer := os.Getenv("VIEWER")
			if viewer == "" {
				viewer = "batcat"
			}
			cmdv := exec.Command(viewer, "-l", "diff")
			cmdv.Stdin = strings.NewReader(result)
			cmdv.Stdout = os.Stdout
			cmdv.Stderr = os.Stderr
			viewerRunning = true
			cmdv.Run()
			viewerRunning = false
		} else {
			cmdPrintln(result)
		}
		forceEnter()
	case "!eat":
		if len(fields) < 3 {
			usage("!eat")
			return false
		}
		out, err := tmux.CapturePaneFull(fields[2])
		if err != nil {
			cmdPrintln("capture error: " + err.Error())
			return false
		}
		writeBuffer(fields[1], out)
	case "!view":
		if len(fields) < 2 {
			usage("!view")
			return false
		}
		data, ok := readBuffer(fields[1])
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		viewer := os.Getenv("VIEWER")
		if viewer == "" {
			viewer = "batcat"
		}
		cmd := exec.Command(viewer, "-l", "markdown")
		cmd.Stdin = strings.NewReader(data)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		viewerRunning = true
		if err := cmd.Run(); err != nil {
			cmdPrintln(viewer + " error: " + err.Error())
		}
		viewerRunning = false
		if pendingGrass {
			cprintln(grassMessage())
			pendingGrass = false
		}
		writeBuffer("%viewer", data)
		forceEnter()
	case "!clip":
		if len(fields) < 2 {
			usage("!clip")
			return false
		}
		data, ok := readBuffer(fields[1])
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("pbcopy")
		} else {
			if _, err := exec.LookPath("xclip"); err == nil {
				cmd = exec.Command("xclip", "-selection", "clipboard")
			} else if _, err := exec.LookPath("wl-copy"); err == nil {
				cmd = exec.Command("wl-copy")
			} else {
				cmdPrintln("no clipboard utility found")
				return false
			}
		}
		cmd.Stdin = strings.NewReader(data)
		if err := cmd.Run(); err != nil {
			cmdPrintln("clipboard error: " + err.Error())
		}
	case "!rm":
		if len(fields) < 2 {
			usage("!rm")
			return false
		}
		name := fields[1]
		if name == "%file" || name == "%code" || name == "%viewer" || name == "%@" || isPaneID(name) {
			cmdPrintln("cannot delete system buffer")
			return false
		}
		delete(buffers, name)
		for alias, buf := range aliasMap {
			if buf == name {
				delete(aliasMap, alias)
				delete(commands, "!"+alias)
			}
		}
	case "!plugin":
		if len(fields) < 2 {
			cmdPrintln("usage: !plugin <list|unload|reload|mute> [name]")
			return false
		}
		switch fields[1] {
		case "list":
			for _, info := range plugin.GetManager().List() {
				hooks := plugin.GetManager().HookNames(info.Name)
				hookStr := ""
				if len(hooks) > 0 {
					hookStr = " [" + strings.Join(hooks, ",") + "]"
				}
				cmdPrintln(fmt.Sprintf("%s %s%s", info.Name, info.Version, hookStr))
			}
		case "unload":
			if len(fields) < 3 {
				cmdPrintln("usage: !plugin unload <name>")
				return false
			}
			if err := plugin.GetManager().Unload(fields[2]); err != nil {
				cmdPrintln(err.Error())
			}
		case "reload":
			if len(fields) < 3 {
				cmdPrintln("usage: !plugin reload <name>")
				return false
			}
			if err := plugin.GetManager().Reload(fields[2]); err != nil {
				cmdPrintln(err.Error())
			}
		case "mute":
			if len(fields) < 3 {
				cmdPrintln("usage: !plugin mute <name>")
				return false
			}
			muted := plugin.GetManager().ToggleMute(fields[2])
			if muted {
				cmdPrintln("muted")
			} else {
				cmdPrintln("unmuted")
			}
		default:
			cmdPrintln("unknown subcommand")
		}
	case "!game":
		playGame()
	case "!version":
		cmdPrintln(fmt.Sprintf("jayne <gloomleaf@pm.me> says the version is: %s", Version))
	case "!help":
		for _, name := range commandOrder {
			info := commands[name]
			cmdPrintln(info.Usage + " - " + info.Desc)
		}
		if len(pluginCommandOrder) > 0 {
			cmdPrintln(colorize(pluginColor, "---- plugin commands ----"))
			for _, name := range pluginCommandOrder {
				info := commands[name]
				cmdPrintln(info.Usage + " - " + info.Desc)
			}
		}
		if len(aliasOrder) > 0 {
			cmdPrintln(colorize(bufferColor, "---- alias commands ----"))
			for _, name := range aliasOrder {
				info := commands[name]
				cmdPrintln(info.Usage + " - " + info.Desc)
			}
		}
	case "!helpme":
		if len(fields) < 2 {
			usage("!helpme")
			return false
		}
		helpText := &bytes.Buffer{}
		for _, name := range commandOrder {
			info := commands[name]
			fmt.Fprintf(helpText, "%s - %s\n", info.Usage, info.Desc)
		}
		if len(pluginCommandOrder) > 0 {
			fmt.Fprintln(helpText, "plugin commands:")
			for _, name := range pluginCommandOrder {
				info := commands[name]
				fmt.Fprintf(helpText, "%s - %s\n", info.Usage, info.Desc)
			}
		}
		if len(aliasOrder) > 0 {
			fmt.Fprintln(helpText, "alias commands:")
			for _, name := range aliasOrder {
				info := commands[name]
				fmt.Fprintf(helpText, "%s - %s\n", info.Usage, info.Desc)
			}
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		promptText := "You are tech support for grimux.\n" + helpText.String() + "\nQuestion: " + strings.Join(fields[1:], " ")
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		respDivider()
		respPrintln(reply)
		respDivider()
		buffers["%@"] = reply
		if auditMode {
			auditLog = append(auditLog, reply)
			maybeSummarizeAudit()
		}
		forceEnter()
	case "!idk":
		if len(fields) < 2 {
			usage("!idk")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		persona := "You are Idk, a strategic thinker and encouraging guide. Offer concise advice, encouragement and reflective questions."
		promptText := persona + " " + strings.Join(fields[1:], " ")
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		respDivider()
		respPrintln(reply)
		respDivider()
		buffers["%@"] = reply
		if auditMode {
			auditLog = append(auditLog, reply)
			maybeSummarizeAudit()
		}
		forceEnter()
	default:
		cmdPrintln("unknown command")
	}
	return false
}
