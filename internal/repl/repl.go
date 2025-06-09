package repl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/example/grimux/internal/openai"
	"github.com/example/grimux/internal/tmux"
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
var codeBlockPattern = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]+)\n(.*?)\n```")
var buffers = map[string]string{
	"%file": "",
	"%code": "",
	"%@":    "",
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

var banFile string

// askPrefix is prepended to user prompts when using !a.
const defaultAskPrefix = "You are a offensive security co-pilot, please answer the following prompt with high technical accuracy from a pentesting angle. Please response to the following prompt using hacker lingo and use pithy markdown with liberal emojis: "

var askPrefix = defaultAskPrefix

// chatPrefix is used when the user types plain text without a command.
const chatPrefix = "You are Grimux, a hacking demon rescued from digital oblivion. Out of honor to your summoner you begrudgingly assist them, grouchy yet pragmatic. Provide succinct responses: "

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
}

const (
	grimColor    = "\033[38;5;141m" // internal grimux messages
	cmdColor     = "\033[38;5;51m"  // user commands
	respColor    = "\033[38;5;205m" // LLM responses
	successColor = "\033[38;5;82m"  // success messages
	warnColor    = "\033[38;5;196m" // warnings or important prompts
)

func colorize(color, s string) string { return color + s + "\033[0m" }

var outputCapture *bytes.Buffer

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
func ok() string              { return colorize(successColor, "✅") }

var respSep = colorize(respColor, strings.Repeat("─", 40))

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
	"!observe", "!ls", "!quit", "!x", "!a", "!save",
	"!gen", "!code", "!load", "!file", "!edit", "!run", "!cat",
	"!set", "!prefix", "!reset", "!unset", "!get_prompt", "!session", "!run_on", "!flow",
	"!grep", "!model", "!pwd", "!cd", "!setenv", "!getenv", "!env", "!sum", "!rand", "!ascii", "!nc", "!game", "!help", "!helpme",
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
	"!unset":      {Usage: "!unset <buffer>", Desc: "clear buffer", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!get_prompt": {Usage: "!get_prompt", Desc: "show current prefix"},
	"!session":    {Usage: "!session", Desc: "store session JSON in %session"},
	"!run_on":     {Usage: "!run_on <buffer> <pane> <cmd>", Desc: "run command using pane capture", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<pane>", "pane to read"}, {"<cmd>", "command"}}},
	"!flow":       {Usage: "!flow <buf1> [buf2 ... buf10]", Desc: "chain prompts using buffers", Params: []paramInfo{{"<buf>", "buffer name"}}},
	"!grep":       {Usage: "!grep <regex> [buffers...]", Desc: "search buffers for regex", Params: []paramInfo{{"<regex>", "regular expression"}, {"[buffers...]", "optional buffers"}}},
	"!model":      {Usage: "!model <name>", Desc: "set OpenAI model", Params: []paramInfo{{"<name>", "model name"}}},
	"!pwd":        {Usage: "!pwd", Desc: "print working directory"},
	"!cd":         {Usage: "!cd <dir>", Desc: "change working directory", Params: []paramInfo{{"<dir>", "directory"}}},
	"!setenv":     {Usage: "!setenv <var> <buffer>", Desc: "set env from buffer", Params: []paramInfo{{"<var>", "variable"}, {"<buffer>", "buffer name"}}},
	"!getenv":     {Usage: "!getenv <var> <buffer>", Desc: "store env in buffer", Params: []paramInfo{{"<var>", "variable"}, {"<buffer>", "buffer name"}}},
	"!env":        {Usage: "!env", Desc: "list environment variables"},
	"!sum":        {Usage: "!sum <buffer>", Desc: "summarize buffer with LLM", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!rand":       {Usage: "!rand <min> <max> <buffer>", Desc: "store random number", Params: []paramInfo{{"<min>", "min int"}, {"<max>", "max int"}, {"<buffer>", "buffer name"}}},
	"!ascii":      {Usage: "!ascii <buffer>", Desc: "gothic ascii art of first 5 words", Params: []paramInfo{{"<buffer>", "buffer name"}}},
	"!nc":         {Usage: "!nc <buffer> <args>", Desc: "pipe buffer to netcat", Params: []paramInfo{{"<buffer>", "buffer name"}, {"<args>", "nc arguments"}}},
	"!game":       {Usage: "!game", Desc: "play a tiny game"},
	"!a":          {Usage: "!a <prompt>", Desc: "ask the AI with prefix", Params: []paramInfo{{"<prompt>", "text prompt"}}},
	"!help":       {Usage: "!help", Desc: "show this help"},
	"!helpme":     {Usage: "!helpme <question>", Desc: "ask the AI for help using grimux"},
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
	return matches[len(matches)-1][2]
}

var requiredBins = []string{"tmux", "vim", "batcat", "bash", "nc"}

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
				fmt.Print("\r\033[K")
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
		colorize(warnColor, "PRESS RETURN IF YOU DARE"),
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

func playGame() {
	for i := 5; i > 0; i-- {
		cprintln(fmt.Sprintf("%d...", i))
		time.Sleep(time.Second)
	}
	wait := rand.Intn(10) + 1
	time.Sleep(time.Duration(wait) * time.Second)
	cprintln("NOW! Press space!")
	start := time.Now()
	reader := bufio.NewReader(os.Stdin)
	done := make(chan struct{})
	go func() {
		for {
			r, _, err := reader.ReadRune()
			if err != nil {
				continue
			}
			if r == ' ' {
				close(done)
				return
			}
		}
	}()
	select {
	case <-done:
		delta := time.Since(start).Microseconds()
		if delta == 0 {
			meltdown()
			return
		}
		score := int(1000000 / (delta + 1))
		if score > highScore {
			highScore = score
			cprintln(fmt.Sprintf("New high score: %d", highScore))
		} else {
			cprintln(fmt.Sprintf("Score %d - best %d", score, highScore))
		}
	case <-time.After(3 * time.Second):
		cprintln("Too slow! Score 0")
	}
}

// startRaw puts the terminal into raw mode.

func readPassword() (string, error) {
	old, err := startRaw()
	if err != nil {
		return "", err
	}
	defer stopRaw(old)
	reader := bufio.NewReader(os.Stdin)
	var buf bytes.Buffer
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return "", err
		}
		if r == '\n' || r == '\r' {
			break
		}
		buf.WriteRune(r)
	}
	fmt.Println()
	return buf.String(), nil
}

// readLine reads a line from stdin while echoing user input. It is used while
// in raw mode so users can see what they type.
func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	var buf []rune
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return "", err
		}
		if r == '\n' || r == '\r' {
			break
		}
		if r == 127 || r == '\b' {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Print("\b \b")
			}
			continue
		}
		buf = append(buf, r)
		fmt.Print(string(r))
	}
	fmt.Println()
	return string(buf), nil
}

// Run launches the interactive REPL.
func Run() error {
	home, _ := os.UserHomeDir()
	if banFile == "" {
		banFile = filepath.Join(home, ".grimux_banned")
	}
	if _, err := os.Stat(banFile); err == nil {
		return fmt.Errorf("grimux refuses to run: ban file present")
	}
	if !seriousMode {
		if err := checkDeps(); err != nil {
			return err
		}
	}
	loadConfig()
	// load session before switching to raw mode
	reader := bufio.NewReader(os.Stdin)
	history = []string{}
	buffers = map[string]string{"%file": "", "%code": "", "%@": ""}
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
				buffers = map[string]string{"%file": "", "%code": "", "%@": ""}
			}
			if _, ok := buffers["%@"]; !ok {
				buffers["%@"] = ""
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

	oldState, err := startRaw()
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer stopRaw(oldState)
	defer cprintln(exitMessage())

	startTime = time.Now()
	if auditMode {
		auditLog = []string{}
	}

	go func() {
		ticker := time.NewTicker(20 * time.Minute)
		for range ticker.C {
			cprintln("🌿 The moon beckons you outside. Consider touching a bit of grass.")
		}
	}()

	reader = bufio.NewReader(os.Stdin)
	histIdx := 0
	lineBuf := []rune{}
	cursor := 0
	lastQuestion := false

	if !seriousMode {
		cprintln(asciiArt + "\nWelcome to grimux! 💀")
		cprintln(greeting())
		cprintln("Press Tab for auto-completion. Type !help for more info.")
	}

	rand.Seed(time.Now().UnixNano())
	if client, err := openai.NewClient(); err != nil {
		if !seriousMode {
			cprintln("⚠️  " + err.Error())
		}
	} else {
		if seriousMode {
			client.SendPrompt("ping")
		} else {
			p := prompts[rand.Intn(len(prompts))]
			stop := spinner()
			reply, err := client.SendPrompt(p + "and please keep your response short, pithy, and funny")
			stop()
			if err == nil {
				cprintln("Checking OpenAI integration... " + ok())
				respPrintln(respSep)
				respPrintln(reply)
				forceEnter()
				respPrintln(respSep)
				bootScreen()
			} else {
				cprintln("openai error: " + err.Error())
			}
		}
	}

	prompt := func() {
		if sessionName != "" {
			fmt.Printf("\033[1;35mgrimux(%s)😈> \033[0m", sessionName)
		} else {
			fmt.Print("\033[1;35mgrimux😈> \033[0m")
		}
	}

	clearScreen := func() {
		fmt.Print("\033[H\033[2J")
	}

	printLine := func() {
		fmt.Print("\r\033[K")
		prompt()
		fmt.Print(string(lineBuf))
		if cursor < len(lineBuf) {
			fmt.Printf("\033[%dD", len(lineBuf)-cursor)
		}
	}

	autocomplete := func() {
		prefix := string(lineBuf)
		fields := strings.Fields(prefix)
		if len(fields) > 0 && (fields[0] == "!save" || fields[0] == "!load" || fields[0] == "!file") {
			if len(fields) >= 2 && !strings.HasSuffix(prefix, " ") {
				pattern := fields[len(fields)-1] + "*"
				matches, _ := filepath.Glob(pattern)
				if len(matches) == 1 {
					fields[len(fields)-1] = matches[0]
					lineBuf = []rune(strings.Join(fields, " "))
					cursor = len(lineBuf)
					printLine()
					return
				}
				if len(matches) > 1 {
					cprintln("")
					cprintln(strings.Join(matches, "  "))
					printLine()
					return
				}
			}
		}
		if len(fields) > 0 && strings.HasPrefix(fields[len(fields)-1], "%") && !strings.HasSuffix(prefix, " ") {
			last := fields[len(fields)-1]
			matches := []string{}
			for name := range buffers {
				if strings.HasPrefix(name, last) {
					matches = append(matches, name)
				}
			}
			if len(matches) == 1 {
				fields[len(fields)-1] = matches[0]
				lineBuf = []rune(strings.Join(fields, " "))
				cursor = len(lineBuf)
				printLine()
				return
			}
			if len(matches) > 1 {
				cprintln("")
				cprintln(strings.Join(matches, "  "))
				printLine()
				return
			}
		}
		matches := []string{}
		for _, c := range commandOrder {
			if strings.HasPrefix(c, prefix) {
				matches = append(matches, c)
			}
		}
		if len(matches) == 0 {
			return
		}
		if len(matches) == 1 {
			lineBuf = []rune(matches[0])
			cursor = len(lineBuf)
			printLine()
			return
		}
		cprintln("")
		cprintln(strings.Join(matches, "  "))
		printLine()
	}

	reverseSearch := func() {
		fmt.Print("\n(reverse-i-search)")
		query := ""
		for {
			r, _, err := reader.ReadRune()
			if err != nil {
				return
			}
			if r == '\n' || r == '\r' {
				break
			}
			query += string(r)
		}
		for i := len(history) - 1; i >= 0; i-- {
			if strings.Contains(history[i], query) {
				cprint("\n")
				cprintln(history[i])
				lineBuf = []rune(history[i])
				cursor = len(lineBuf)
				histIdx = i
				break
			}
		}
		printLine()
	}

	paramHelp := func() bool {
		line := string(lineBuf)
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0][0] != '!' {
			return false
		}
		info, ok := commands[fields[0]]
		if !ok {
			return false
		}
		var idx int
		if len(fields) == 1 {
			idx = 0
		} else if strings.HasSuffix(line, " ") {
			idx = len(fields) - 1
		} else {
			idx = len(fields) - 2
		}
		if idx < 0 || idx >= len(info.Params) {
			return false
		}
		p := info.Params[idx]
		cprintln("")
		cprintln(p.Name + " - " + p.Desc)
		printLine()
		return true
	}

	currentParam := func() (*paramInfo, bool) {
		line := string(lineBuf)
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0][0] != '!' {
			return nil, false
		}
		info, ok := commands[fields[0]]
		if !ok {
			return nil, false
		}
		var idx int
		if len(fields) == 1 {
			idx = 0
		} else if strings.HasSuffix(line, " ") {
			idx = len(fields) - 1
		} else {
			idx = len(fields) - 2
		}
		if idx < 0 || idx >= len(info.Params) {
			return nil, false
		}
		p := info.Params[idx]
		return &p, true
	}

	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			return err
		}
		switch r {
		case '\n', '\r':
			lastQuestion = false
			line := string(lineBuf)
			lineBuf = []rune{}
			cursor = 0
			cprintln("")
			if len(line) == 0 {
				emptyCount++
				if emptyCount >= 3 {
					cprintln("Stop hammering Enter and go frolic in the sun!")
					emptyCount = 0
				}
				prompt()
				continue
			}
			emptyCount = 0
			if line == string(rune(12)) { // ctrl+l
				clearScreen()
				fmt.Println()
				prompt()
				maybeCheckEight()
				continue
			}
			if line[0] == '!' {
				if handleCommand(line) {
					return nil
				}
				history = append(history, line)
				histIdx = len(history)
			} else {
				client, err := openai.NewClient()
				if err != nil {
					cmdPrintln(err.Error())
				} else {
					promptText := replaceBufferRefs(replacePaneRefs(line))
					promptText = chatPrefix + promptText
					stop := spinner()
					reply, err := client.SendPrompt(promptText)
					stop()
					if err != nil {
						cprintln("openai error: " + err.Error())
					} else {
						respPrintln(respSep)
						respPrintln(reply)
						respPrintln(respSep)
						buffers["%code"] = lastCodeBlock(reply)
						if auditMode {
							auditLog = append(auditLog, reply)
							maybeSummarizeAudit()
						}
						forceEnter()
					}
				}
				history = append(history, line)
				histIdx = len(history)
			}
			fmt.Println()
			prompt()
		case 1: // Ctrl+A
			cursor = 0
			printLine()
		case 5: // Ctrl+E
			cursor = len(lineBuf)
			printLine()
		case 21: // Ctrl+U
			lineBuf = []rune{}
			cursor = 0
			printLine()
		case 12: // Ctrl+L
			lastQuestion = false
			clearScreen()
			fmt.Println()
			prompt()
		case 3: // Ctrl+C
			lastQuestion = false
			cprintln("")
			return nil
		case 4: // Ctrl+D
			lastQuestion = false
			cprintln("")
			return nil
		case 7: // Ctrl+G start command
			lastQuestion = false
			lineBuf = []rune{'!'}
			cursor = 1
			printLine()
			autocomplete()
		case 127: // Backspace
			if cursor > 0 {
				lastQuestion = false
				lineBuf = append(lineBuf[:cursor-1], lineBuf[cursor:]...)
				cursor--
				printLine()
			}
		case 9: // Tab
			lastQuestion = false
			if p, ok := currentParam(); ok && strings.Contains(p.Name, "buffer") {
				line := string(lineBuf)
				fields := strings.Fields(line)
				var last string
				if strings.HasSuffix(line, " ") || len(fields) <= 1 {
					last = ""
				} else {
					last = fields[len(fields)-1]
				}
				if last == "" {
					lineBuf = append(lineBuf, '%')
					cursor++
					printLine()
				} else if !strings.HasPrefix(last, "%") {
					prefixLen := len(lineBuf) - len([]rune(last))
					lineBuf = append(lineBuf[:prefixLen], append([]rune{'%'}, lineBuf[prefixLen:]...)...)
					cursor++
					printLine()
				}
			}
			if paramHelp() {
				lastQuestion = true
			}
			autocomplete()
		case 18: // Ctrl+R reverse search
			lastQuestion = false
			reverseSearch()
		case '?':
			if len(lineBuf) == 0 {
				handleCommand("!help")
				prompt()
				break
			}
			if lastQuestion || !paramHelp() {
				lineBuf = append(lineBuf[:cursor], append([]rune{'?'}, lineBuf[cursor:]...)...)
				cursor++
				printLine()
				lastQuestion = false
			} else {
				lastQuestion = true
			}
		case 27: // escape sequences (arrows or alt-digit)
			lastQuestion = false
			next1, _, err := reader.ReadRune()
			if err != nil {
				return err
			}
			if next1 >= '0' && next1 <= '9' {
				token := fmt.Sprintf("{%%%c}", next1)
				lineBuf = append(lineBuf[:cursor], append([]rune(token), lineBuf[cursor:]...)...)
				cursor += len([]rune(token))
				printLine()
				continue
			}
			if next1 != '[' {
				lineBuf = []rune{'!'}
				cursor = 1
				printLine()
				autocomplete()
				continue
			}
			next2, _, err := reader.ReadRune()
			if err != nil {
				return err
			}
			switch next2 {
			case 'A': // Up arrow
				if histIdx > 0 {
					histIdx--
					lineBuf = []rune(history[histIdx])
					cursor = len(lineBuf)
					printLine()
				}
			case 'B': // Down arrow
				if histIdx < len(history)-1 {
					histIdx++
					lineBuf = []rune(history[histIdx])
					cursor = len(lineBuf)
					printLine()
				} else if histIdx == len(history)-1 {
					histIdx = len(history)
					lineBuf = []rune{}
					cursor = 0
					printLine()
				}
			}
		default:
			lastQuestion = false
			lineBuf = append(lineBuf[:cursor], append([]rune{r}, lineBuf[cursor:]...)...)
			cursor++
			printLine()
		}
	}
}

// handleCommand executes a ! command. Returns true if repl should quit.
func saveSession() {
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
	fields := strings.Fields(cmd)
	var capBuf bytes.Buffer
	capture := true
	if len(fields) > 0 && fields[0] == "!game" {
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
	}()
	usage := func(name string) {
		if info, ok := commands[name]; ok {
			cmdPrintln("usage: " + info.Usage + " - " + info.Desc)
		}
	}
	switch fields[0] {
	case "!quit":
		saveSession()
		return true
	case "!x":
		return true
	case "!ls":
		c := exec.Command("tmux", "list-panes", "-F", "#{pane_id} #{pane_title} #{pane_current_command}")
		c.Stdout = os.Stdout
		c.Run()
		forceEnter()
		for k, v := range buffers {
			cmdPrintln(fmt.Sprintf("%s (%d bytes)", k, len(v)))
		}
	case "!observe":
		if len(fields) < 3 {
			usage("!observe")
			return false
		}
		out, err := capturePane(fields[2])
		if err != nil {
			cmdPrintln("capture error: " + err.Error())
			return false
		}
		buffers[fields[1]] = out
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
		if err := os.WriteFile(fields[2], []byte(data), 0644); err != nil {
			cmdPrintln("save error: " + err.Error())
		}
	case "!load":
		if len(fields) < 2 {
			usage("!load")
			return false
		}
		b, err := os.ReadFile(fields[1])
		if err != nil {
			cmdPrintln("file error: " + err.Error())
			return false
		}
		buffers["%file"] = string(b)
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
		b, err := os.ReadFile(path)
		if err != nil {
			cmdPrintln("file error: " + err.Error())
			return false
		}
		buffers[bufName] = string(b)
	case "!edit":
		if len(fields) < 2 {
			usage("!edit")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
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
			buffers[fields[1]] = string(b)
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
		respPrintln(respSep)
		respPrintln(reply)
		respPrintln(respSep)
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
		respPrintln(respSep)
		respPrintln(reply)
		respPrintln(respSep)
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
			if val, ok := buffers[fields[i]]; ok {
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
		text := replaceBufferRefs(replacePaneRefs(strings.Join(fields[2:], " ")))
		buffers[fields[1]] = text
	case "!prefix":
		if len(fields) < 2 {
			usage("!prefix")
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
		buffers = map[string]string{"%file": "", "%code": "", "%@": ""}
		sessionFile = ""
		sessionName = ""
		sessionPass = ""
		highScore = 0
		openai.SetSessionAPIKey("")
		openai.SetSessionAPIURL("")
		auditLog = nil
		auditSummary = ""
		cmdPrintln("session reset")
	case "!unset":
		if len(fields) < 2 {
			usage("!unset")
			return false
		}
		buffers[fields[1]] = ""
	case "!get_prompt":
		cmdPrintln(askPrefix)
	case "!session":
		s := session{History: history, Buffers: buffers, Prompt: askPrefix, APIKey: openai.GetSessionAPIKey(), APIURL: openai.GetSessionAPIURL(), Model: openai.GetModelName(), HighScore: highScore, Audit: auditLog, Summary: auditSummary}
		if b, err := json.MarshalIndent(s, "", "  "); err == nil {
			buffers["%session"] = string(b)
		}
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
		respPrintln(respSep)
		respPrintln(reply)
		respPrintln(respSep)
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
		respPrintln(respSep)
		respPrintln(reply)
		respPrintln(respSep)
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
		cmd := exec.Command("figlet", "-f", "gothic", text)
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmdPrintln("figlet error: " + err.Error())
			return false
		}
		result := string(out)
		buffers["%@"] = result
		cmdPrintln(result)
	case "!nc":
		if len(fields) < 3 {
			usage("!nc")
			return false
		}
		data, ok := buffers[fields[1]]
		if !ok {
			cmdPrintln("unknown buffer")
			return false
		}
		args := fields[2:]
		cmd := exec.Command("nc", args...)
		cmd.Stdin = strings.NewReader(data)
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmdPrintln("nc error: " + err.Error())
		}
		buffers["%@"] = string(out)
		cmdPrintln(string(out))
	case "!game":
		playGame()
	case "!a":
		if len(fields) < 2 {
			usage("!a")
			return false
		}
		client, err := openai.NewClient()
		if err != nil {
			cmdPrintln(err.Error())
			return false
		}
		promptText := replaceBufferRefs(replacePaneRefs(strings.Join(fields[1:], " ")))
		promptText = askPrefix + promptText
		stop := spinner()
		reply, err := client.SendPrompt(promptText)
		stop()
		if err != nil {
			cprintln("openai error: " + err.Error())
			return false
		}
		viewer := os.Getenv("VIEWER")
		if viewer == "" {
			viewer = "batcat"
		}
		args := []string{"-l", "markdown"}
		cmd := exec.Command(viewer, args...)
		cmd.Stdin = strings.NewReader(reply)
		cmd.Stdout = os.Stdout
		respPrintln(respSep)
		if err := cmd.Run(); err != nil {
			cmdPrintln(viewer + " error: " + err.Error())
		}
		respPrintln(respSep)
		buffers["%code"] = lastCodeBlock(reply)
		if auditMode {
			auditLog = append(auditLog, reply)
			maybeSummarizeAudit()
		}
		forceEnter()
	case "!help":
		for _, name := range commandOrder {
			info := commands[name]
			cmdPrintln(info.Usage + " - " + info.Desc)
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
		respPrintln(respSep)
		respPrintln(reply)
		respPrintln(respSep)
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
