//go:build webui
// +build webui

package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed all:dist
var distFS embed.FS

var webuiPort = getEnvPort("GOLC_WEBUI_PORT", 8091)
var resultsAllPort = getEnvPort("GOLC_RESULTS_PORT", 8090)

func getEnvPort(envKey string, defaultVal int) int {
	if s := os.Getenv(envKey); s != "" {
		if p, err := strconv.Atoi(s); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	return defaultVal
}

// ─── State ───────────────────────────────────────────────────────────────────

type Phase string

const (
	PhaseIdle       Phase = "idle"
	PhaseIdentify   Phase = "identifying"
	PhaseAnalyzing  Phase = "analyzing"
	PhaseReporting  Phase = "reporting"
	PhaseComplete   Phase = "complete"
	PhaseError      Phase = "error"
)

const (
	contentTypeHeader   = "Content-Type"
	contentTypeJSON     = "application/json"
	errPostOnly         = "POST only"
	labelIdentifyingFmt = "Identifying repositories (%d/%d)"
)

type ProgressEvent struct {
	Type    string `json:"type"`             // "progress" | "log" | "complete" | "error"
	Message string `json:"message"`          // displayed in log terminal (original log line when available)
	Label   string `json:"label,omitempty"`  // progress-bar label; falls back to Message when empty
	Phase   Phase  `json:"phase"`
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Pct     int    `json:"pct"`
}

type AppState struct {
	mu          sync.RWMutex
	running     bool
	phase       Phase
	current     int
	total       int
	errMsg      string
	eventBuf    []ProgressEvent
	clients     []chan ProgressEvent
	golcCmd     *exec.Cmd
	resultsProc *os.Process
	resultsPort int // port the current ResultsAll instance is bound to
}

var appState = &AppState{phase: PhaseIdle}

func (s *AppState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.phase = PhaseIdle
	s.current = 0
	s.total = 0
	s.errMsg = ""
	s.eventBuf = nil
}

func (s *AppState) addClient(ch chan ProgressEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients = append(s.clients, ch)
}

func (s *AppState) removeClient(ch chan ProgressEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.clients {
		if c == ch {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			return
		}
	}
}

func (s *AppState) broadcast(ev ProgressEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventBuf = append(s.eventBuf, ev)
	// cap buffer at 500 entries
	if len(s.eventBuf) > 500 {
		s.eventBuf = s.eventBuf[len(s.eventBuf)-500:]
	}
	for _, ch := range s.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

// ─── Config helpers ───────────────────────────────────────────────────────────

// platformDefaults provides the non-user-facing defaults for each platform.
var platformDefaults = map[string]map[string]interface{}{
	"Github": {
		"DevOps": "github", "Url": "https://api.github.com/", "Apiver": "2022-11-28",
		"Baseapi": "github.com", "Protocol": "https", "FileExclusion": ".cloc_github_ignore",
		"Multithreading": true, "Workers": float64(10), "NumberWorkerRepos": float64(10),
		"ResultAll": true, "Org": true, "Period": float64(-1), "Factor": float64(33),
		"DefaultBranch": true, "Stats": false, "ResultByFile": true,
	},
	"GithubEnterprise": {
		"DevOps": "github", "Apiver": "2022-11-28", "FileExclusion": ".cloc_github_ignore",
		"Multithreading": true, "Workers": float64(10), "NumberWorkerRepos": float64(10),
		"ResultAll": true, "Org": true, "Period": float64(-1), "Factor": float64(33),
		"DefaultBranch": true, "Stats": false, "ResultByFile": true,
	},
	"Gitlab": {
		"DevOps": "gitlab", "Url": "https://gitlab.com/", "Apiver": "v4",
		"Baseapi": "api/", "Protocol": "https", "FileExclusion": ".cloc_gitlab_ignore",
		"Multithreading": true, "Workers": float64(10), "NumberWorkerRepos": float64(10),
		"ResultAll": true, "Org": true, "Period": float64(-1), "Factor": float64(33),
		"DefaultBranch": true, "Stats": false, "ResultByFile": true,
	},
	"BitBucket": {
		"DevOps": "bitbucket", "Url": "https://api.bitbucket.org/", "Apiver": "2.0",
		"Baseapi": "bitbucket.org", "Protocol": "https", "FileExclusion": ".cloc_bitbucket_ignore",
		"Multithreading": true, "Workers": float64(10), "NumberWorkerRepos": float64(10),
		"ResultAll": true, "Org": true, "Period": float64(-1), "Factor": float64(33),
		"DefaultBranch": true, "Stats": false, "ResultByFile": true,
	},
	"BitBucketSRV": {
		"DevOps": "bitbucket_dc", "Apiver": "1.0", "Baseapi": "rest/api/",
		"FileExclusion": ".cloc_bitbucketdc_ignore",
		"Multithreading": true, "Workers": float64(10), "NumberWorkerRepos": float64(10),
		"ResultAll": true, "Org": true, "Period": float64(-5), "Factor": float64(33),
		"DefaultBranch": true, "Stats": false, "ResultByFile": true,
	},
	"Azure": {
		"DevOps": "azure", "Url": "https://dev.azure.com/", "Apiver": "7.1",
		"Baseapi": "_apis/git/", "Protocol": "https", "FileExclusion": ".cloc_azure_ignore",
		"Multithreading": true, "Workers": float64(10), "NumberWorkerRepos": float64(10),
		"ResultAll": true, "Org": true, "Period": float64(-1), "Factor": float64(33),
		"DefaultBranch": true, "Stats": false, "ResultByFile": true,
	},
	"File": {
		"DevOps": "file", "FileExclusion": ".cloc_file_ignore", "FileLoad": ".cloc_file_load",
		"ResultAll": true, "ResultByFile": true, "ScanSubDirs": true,
	},
}

func loadFullConfig() (map[string]interface{}, error) {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	return cfg, json.Unmarshal(data, &cfg)
}

func getPlatformConfig(platformKey string) (map[string]interface{}, error) {
	full, err := loadFullConfig()
	if err != nil {
		return nil, err
	}
	platforms, _ := full["platforms"].(map[string]interface{})
	if platforms == nil {
		return map[string]interface{}{}, nil
	}
	pc, _ := platforms[platformKey].(map[string]interface{})
	if pc == nil {
		return map[string]interface{}{}, nil
	}
	return pc, nil
}

func savePlatformConfig(platformKey string, platformCfg map[string]interface{}) error {
	full, err := loadFullConfig()
	if err != nil {
		// If config.json doesn't exist yet, build a minimal one
		full = map[string]interface{}{
			"platforms": map[string]interface{}{},
			"Logging":   map[string]interface{}{"Level": "debug"},
			"Release":   map[string]interface{}{"Version": "2.0"},
		}
	}
	platforms, ok := full["platforms"].(map[string]interface{})
	if !ok {
		platforms = map[string]interface{}{}
		full["platforms"] = platforms
	}

	// Merge defaults then user values
	merged := map[string]interface{}{}
	for k, v := range platformDefaults[platformKey] {
		merged[k] = v
	}
	for k, v := range platformCfg {
		merged[k] = v
	}
	platforms[platformKey] = merged

	data, err := json.MarshalIndent(full, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile("config.json", data, 0644)
}

// ─── Progress parsing ─────────────────────────────────────────────────────────

var (
	// Matches total repo/project/branch count for all platforms:
	//   GitHub/BB/Azure: "Total Repositories that will be analyzed: N"
	//   GitLab:          "TotalProject(s) that will be analyzed: N"
	// Note: per-project "number of Repo(s) found is: N" lines are intentionally
	// excluded here — they are handled by rePerProjectRepoCount below.
	reTotal = regexp.MustCompile(`(?i)(?:number of (?:repositor|director)|TotalProject[^:]*|Total\s+(?:Repositor|Branch|Project)[^:]*)\D*(\d+)`)

	// GitLab only: "✅ Group <name>: N project(s) to analyze" — logged after
	// filterValidProjects so the count reflects only repos that will actually be
	// analyzed (empty/archived excluded). This gives an accurate progress total.
	reGitLabGroup = regexp.MustCompile(`(?i)Group\s*<([^>]+)>:\s*(\d+)\s+project.*?to analyze`)

	// Azure/Bitbucket: "number of Project(s) found/to analyze" fires once before
	// per-project loops begin. Reset the running repo total so it can be rebuilt
	// by accumulating per-project rePerProjectRepoCount matches.
	reProjectFound = regexp.MustCompile(`(?i)number of [Pp]roject\(s\)`)

	// Azure/Bitbucket: "number of Repo(s) found is: N" fires once per project.
	// Accumulate into total so the progress bar denominator grows as each
	// project's repos are discovered (same pattern as reGitLabGroup for GitLab).
	rePerProjectRepoCount = regexp.MustCompile(`(?i)number of Repo\(s\) found is:\s*(\d+)`)

	reAnalyzed = regexp.MustCompile(`(?i)(?:The repository|directory) <([^>]+)> has been analyzed`)

	// Matches per-repo discovery lines across all platforms, capturing multi-word names.
	//   GitHub/BB:  "Repo: name - Targeted/Number of branches:"
	//   BB DC:      "Repo: <name> - Number of branches:"
	//   Azure:      "Repo N: name - Number of branches:"
	//   GitLab:     "N Project: Swag Shop - Number of branches:"
	reRepoDiscover = regexp.MustCompile(`(?i)(?:Repo(?:\s+\d+)?:|(?:\d+\s+)?Project:)\s+<?([^>]+?)>?\s+[-–]`)
)

func normalizeLogMessage(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// parseIntMatch returns the integer in the given capture group if the regex matches and the value is positive.
func parseIntMatch(re *regexp.Regexp, s string, group int) (int, bool) {
	m := re.FindStringSubmatch(s)
	if len(m) <= group {
		return 0, false
	}
	n, err := strconv.Atoi(m[group])
	return n, err == nil && n > 0
}

// extractGroupMatch returns the text of a capture group, or empty string if unmatched.
func extractGroupMatch(re *regexp.Regexp, s string, group int) string {
	m := re.FindStringSubmatch(s)
	if len(m) > group {
		return m[group]
	}
	return ""
}

func applyPerProjectRepoCount(clean string, st *AppState, ev *ProgressEvent) {
	if n, ok := parseIntMatch(rePerProjectRepoCount, clean, 1); ok {
		st.total += n
	}
	ev.Type = "progress"
	ev.Message = normalizeLogMessage(clean)
	ev.Label = fmt.Sprintf(labelIdentifyingFmt, st.current, st.total)
}

func applyGitLabGroupCount(clean string, st *AppState, ev *ProgressEvent) {
	if n, ok := parseIntMatch(reGitLabGroup, clean, 2); ok {
		st.total += n
	}
	ev.Type = "progress"
	ev.Message = normalizeLogMessage(clean)
	ev.Label = fmt.Sprintf(labelIdentifyingFmt, st.current, st.total)
}

func applyRepoDiscovery(clean string, st *AppState, ev *ProgressEvent) {
	st.current++
	repo := extractGroupMatch(reRepoDiscover, clean, 1)
	ev.Type = "progress"
	ev.Message = normalizeLogMessage(clean)
	if repo != "" {
		ev.Label = fmt.Sprintf("Identifying branches: %s (%d/%d)", repo, st.current, st.total)
	} else {
		ev.Label = fmt.Sprintf(labelIdentifyingFmt, st.current, st.total)
	}
}

func applyRepoAnalyzed(clean string, st *AppState, ev *ProgressEvent) {
	st.current++
	repo := extractGroupMatch(reAnalyzed, clean, 1)
	ev.Type = "progress"
	ev.Message = normalizeLogMessage(clean)
	if repo != "" {
		ev.Label = fmt.Sprintf("Counting lines of code: %s (%d/%d)", repo, st.current, st.total)
	} else {
		ev.Label = fmt.Sprintf("Counting lines of code (%d/%d)", st.current, st.total)
	}
}

// applyIdentifyPhaseUpdate handles all sub-cases within the PhaseIdentify stage.
func applyIdentifyPhaseUpdate(clean string, st *AppState, ev *ProgressEvent) {
	switch {
	case reProjectFound.MatchString(clean):
		// Azure/Bitbucket: project count line resets running repo total.
		st.total = 0
		ev.Type = "progress"
		ev.Message = normalizeLogMessage(clean)
		ev.Label = "Identifying repositories..."
	case reTotal.MatchString(clean):
		// All platforms: definitive total once discovery is complete.
		if n, ok := parseIntMatch(reTotal, clean, 1); ok {
			st.total = n
		}
		ev.Type = "progress"
		ev.Message = normalizeLogMessage(clean)
		ev.Label = fmt.Sprintf(labelIdentifyingFmt, st.current, st.total)
	case rePerProjectRepoCount.MatchString(clean):
		applyPerProjectRepoCount(clean, st, ev)
	case reGitLabGroup.MatchString(clean):
		applyGitLabGroupCount(clean, st, ev)
	case reRepoDiscover.MatchString(clean):
		applyRepoDiscovery(clean, st, ev)
	case strings.Contains(clean, "largest repo") || strings.Contains(clean, "largest Repository"):
		ev.Type = "progress"
		ev.Message = normalizeLogMessage(clean)
		ev.Label = "Largest branch identified"
	}
}

// applyProgressCase dispatches the log line to the appropriate phase handler.
// IMPORTANT: phase-transition cases must come BEFORE the catch-all
// "case st.phase == PhaseIdentify" so that lines arriving while in
// PhaseIdentify (e.g. "Analysis of Repos") can still trigger a transition.
func applyProgressCase(clean string, st *AppState, ev *ProgressEvent) {
	switch {
	// ── Phase entry / transition — checked first regardless of current phase ──
	case strings.Contains(clean, "Analysis of devops platform objects") ||
		strings.Contains(clean, "Analysis of Directories"):
		st.phase = PhaseIdentify
		ev.Type = "progress"
		ev.Message = normalizeLogMessage(clean)
		ev.Label = "Identifying repositories..."
	case strings.Contains(clean, "Analysis of Repos"):
		st.phase = PhaseAnalyzing
		st.current = 0
		ev.Type = "progress"
		ev.Message = normalizeLogMessage(clean)
		ev.Label = "Counting lines of code..."
	case strings.Contains(clean, "Analyse Report"):
		st.phase = PhaseReporting
		ev.Type = "progress"
		ev.Message = normalizeLogMessage(clean)
		ev.Label = "Generating global report..."
	case strings.Contains(clean, "Time elapsed"):
		st.phase = PhaseComplete
		ev.Type = "complete"
		ev.Message = normalizeLogMessage(clean)
	// ── Within-phase updates — must come after all transition cases ──
	case st.phase == PhaseIdentify:
		applyIdentifyPhaseUpdate(clean, st, ev)
	case reAnalyzed.MatchString(clean):
		applyRepoAnalyzed(clean, st, ev)
	}
}

func parseProgress(line string, st *AppState) *ProgressEvent {
	clean := stripANSI(line)
	ev := &ProgressEvent{Type: "log", Message: normalizeLogMessage(clean), Phase: st.phase}
	applyProgressCase(clean, st, ev)
	ev.Phase = st.phase
	ev.Current = st.current
	ev.Total = st.total
	ev.Pct = calcPct(st)
	return ev
}

func calcPct(st *AppState) int {
	round := func(f float64) int { return int(math.Round(f)) }
	switch st.phase {
	case PhaseIdle:
		return 0
	case PhaseIdentify:
		if st.total == 0 {
			return 5
		}
		return 5 + round(float64(st.current)/float64(st.total)*30) // 5–35%
	case PhaseAnalyzing:
		if st.total == 0 {
			return 38
		}
		return 38 + round(float64(st.current)/float64(st.total)*56) // 38–94%
	case PhaseReporting:
		return 96
	case PhaseComplete:
		return 100
	case PhaseError:
		return 100
	}
	return 0
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// shouldBroadcastLog returns false for spinner animation frames and for the
// empty timestamp-prefix fragments produced when golc embeds a \r in a log
// message (the custom formatter outputs "[YYYY-MM-DD HH:MM:SS] LEVEL \r<msg>"
// and readLines splits on \r, leaving a fragment with no message content).
func shouldBroadcastLog(line string) bool {
	clean := stripANSI(line)
	// Custom-formatter lines: [2026-04-27 12:12:04] LEVEL message
	if strings.HasPrefix(clean, "[20") {
		// Split into at most 4 parts: ["[date", "time]", "LEVEL", "message"]
		parts := strings.SplitN(clean, " ", 4)
		if len(parts) < 4 || strings.TrimSpace(parts[3]) == "" {
			return false // no message content — discard the empty prefix fragment
		}
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(clean))
	for _, kw := range []string{"waiting for workers", "error", "warning"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// readLines reads from r and calls fn for each non-empty line. '\n' is the
// line terminator; '\r' bytes are silently discarded so that log lines
// containing an embedded \r (e.g. "\r\t✅ Repo: …") are emitted as a single
// complete line rather than split into a bare prefix and a content fragment.
func readLines(r io.Reader, fn func(string)) {
	buf := make([]byte, 32*1024)
	var pending strings.Builder
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			if b == '\n' {
				if pending.Len() > 0 {
					fn(pending.String())
					pending.Reset()
				}
			} else if b != '\r' {
				pending.WriteByte(b)
			}
		}
		if err != nil { // io.EOF or real error
			break
		}
	}
	if pending.Len() > 0 {
		fn(pending.String())
	}
}

// ─── Analysis runner ──────────────────────────────────────────────────────────

func findBinary(name string) (string, error) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	// Check same dir as current executable
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err2 := os.Stat(candidate); err2 == nil {
			return candidate, nil
		}
	}
	// Check CWD
	cwd, _ := os.Getwd()
	candidate := filepath.Join(cwd, name)
	if _, err2 := os.Stat(candidate); err2 == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("binary %q not found", name)
}

// findFreePort returns the preferred port if it is available, otherwise asks
// the OS for any free port.
func findFreePort(preferred int) int {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", preferred))
	if err == nil {
		l.Close()
		return preferred
	}
	// Preferred port is busy — let the OS pick one.
	l, err = net.Listen("tcp", ":0")
	if err != nil {
		return preferred // last resort: try the preferred and let it fail
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// killPort kills any process listening on the given TCP port.
// Uses lsof on macOS/Linux and netstat on Windows.
func killPort(port int) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 200*time.Millisecond)
	if err != nil {
		return // nothing listening
	}
	conn.Close()

	var pids []int
	switch runtime.GOOS {
	case "windows":
		pids = pidsByPortWindows(port)
	default:
		pids = pidsByPortUnix(port)
	}
	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}
}

// pidsByPortUnix uses lsof to find PIDs bound to a TCP port on macOS/Linux.
func pidsByPortUnix(port int) []int {
	portArg := fmt.Sprintf(":%d", port)
	// lsof may live in /usr/sbin (macOS) or /usr/bin (Linux).
	for _, bin := range []string{"/usr/sbin/lsof", "/usr/bin/lsof", "lsof"} {
		out, err := exec.Command(bin, "-ti", portArg).Output()
		if err != nil {
			continue
		}
		return parsePidList(string(out))
	}
	return nil
}

// pidsByPortWindows uses netstat -ano to find PIDs bound to a TCP port on Windows.
func pidsByPortWindows(port int) []int {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return nil
	}
	portSuffix := fmt.Sprintf(":%d", port)
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// Expected format: Proto  LocalAddr  ForeignAddr  State  PID
		if len(fields) < 5 {
			continue
		}
		if !strings.HasSuffix(fields[1], portSuffix) {
			continue
		}
		if strings.ToUpper(fields[3]) != "LISTENING" {
			continue
		}
		if pid, err := strconv.Atoi(fields[4]); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// parsePidList splits whitespace-separated PID output (lsof -t format).
func parsePidList(s string) []int {
	var pids []int
	for _, f := range strings.Fields(s) {
		if pid, err := strconv.Atoi(f); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

func runAnalysis(platformKey string) {
	golcBin, err := os.Executable()
	if err != nil {
		appState.broadcast(ProgressEvent{
			Type:    "error",
			Message: "could not resolve executable path: " + err.Error(),
			Phase:   PhaseError,
		})
		appState.mu.Lock()
		appState.running = false
		appState.phase = PhaseError
		appState.mu.Unlock()
		return
	}

	// Delete Results dir before running analysis
	_ = os.RemoveAll("Results")

	cmd := exec.Command(golcBin, "--internal-run", platformKey)
	cmd.Stdin = strings.NewReader("")

	outR, outW, _ := os.Pipe()
	cmd.Stdout = outW
	cmd.Stderr = outW

	if err := cmd.Start(); err != nil {
		appState.broadcast(ProgressEvent{
			Type:    "error",
			Message: "Failed to start golc: " + err.Error(),
			Phase:   PhaseError,
		})
		appState.mu.Lock()
		appState.running = false
		appState.phase = PhaseError
		appState.golcCmd = nil
		appState.mu.Unlock()
		_ = outW.Close()
		_ = outR.Close()
		return
	}
	outW.Close()
	appState.mu.Lock()
	appState.golcCmd = cmd
	appState.mu.Unlock()

	appState.broadcast(ProgressEvent{
		Type:    "progress",
		Message: "Starting GoLC analysis for platform: " + platformKey,
		Phase:   PhaseIdentify,
		Pct:     2,
	})

	// Local parse state
	parseState := &AppState{phase: PhaseIdentify}

	// Read using raw chunks split on \r or \n so the spinner's \r-terminated
	// animation frames don't accumulate into a single "line" and overflow a
	// bufio.Scanner buffer (default 64 KB → ErrTooLong → pipe closed → SIGPIPE).
	readLines(outR, func(line string) {
		ev := parseProgress(line, parseState)
		appState.mu.Lock()
		appState.phase = parseState.phase
		appState.current = parseState.current
		appState.total = parseState.total
		appState.mu.Unlock()
		// Drop spinner animation frames (type="log" from non-logrus lines).
		// Progress/complete/error events are always forwarded.
		if ev.Type != "log" || shouldBroadcastLog(line) {
			appState.broadcast(*ev)
		}
	})

	outR.Close()
	exitErr := cmd.Wait()

	appState.mu.Lock()
	appState.running = false
	appState.golcCmd = nil
	if exitErr != nil && parseState.phase != PhaseComplete {
		appState.phase = PhaseError
		appState.errMsg = exitErr.Error()
		appState.mu.Unlock()
		appState.broadcast(ProgressEvent{
			Type:    "error",
			Message: "Analysis failed: " + exitErr.Error(),
			Phase:   PhaseError,
			Pct:     100,
		})
	} else {
		appState.phase = PhaseComplete
		appState.mu.Unlock()
		appState.broadcast(ProgressEvent{
			Type:    "complete",
			Message: "Analysis complete. Click 'View Results' to open the dashboard.",
			Phase:   PhaseComplete,
			Pct:     100,
		})
	}
}

// ─── HTTP handlers ────────────────────────────────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("ui").Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), 500)
		return
	}
	w.Header().Set(contentTypeHeader, "text/html; charset=utf-8")
	_ = tmpl.Execute(w, map[string]interface{}{"ResultsAllPort": resultsAllPort})
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		http.Error(w, "missing platform", 400)
		return
	}
	cfg, err := getPlatformConfig(platform)
	if err != nil {
		// Return empty object — no config yet
		cfg = map[string]interface{}{}
	}
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(cfg)
}

type RunRequest struct {
	Platform string                 `json:"platform"`
	Config   map[string]interface{} `json:"config"`
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errPostOnly, 405)
		return
	}

	appState.mu.RLock()
	already := appState.running
	appState.mu.RUnlock()
	if already {
		http.Error(w, "analysis already running", 409)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Platform == "" {
		http.Error(w, "invalid request body", 400)
		return
	}

	if _, ok := platformDefaults[req.Platform]; !ok {
		http.Error(w, "unknown platform", 400)
		return
	}

	// Persist config
	if err := savePlatformConfig(req.Platform, req.Config); err != nil {
		http.Error(w, "failed to save config: "+err.Error(), 500)
		return
	}

	appState.reset()
	appState.mu.Lock()
	appState.running = true
	appState.phase = PhaseIdentify
	appState.mu.Unlock()

	go runAnalysis(req.Platform)

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(contentTypeHeader, "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	ch := make(chan ProgressEvent, 50)
	appState.addClient(ch)
	defer appState.removeClient(ch)

	// Replay buffered events so reconnecting clients catch up
	appState.mu.RLock()
	buffered := make([]ProgressEvent, len(appState.eventBuf))
	copy(buffered, appState.eventBuf)
	appState.mu.RUnlock()

	send := func(ev ProgressEvent) {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	for _, ev := range buffered {
		send(ev)
	}

	// Send a heartbeat every 20s to keep the connection alive
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case ev := <-ch:
			send(ev)
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	appState.mu.RLock()
	defer appState.mu.RUnlock()
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"running": appState.running,
		"phase":   appState.phase,
		"current": appState.current,
		"total":   appState.total,
		"pct":     calcPct(appState),
		"error":   appState.errMsg,
	})
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errPostOnly, 405)
		return
	}
	appState.mu.Lock()
	cmd := appState.golcCmd
	appState.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		http.Error(w, "no analysis running", 409)
		return
	}
	_ = cmd.Process.Kill()
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func handleOpenResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errPostOnly, 405)
		return
	}

	bin, err := findBinary("ResultsAll")

	appState.mu.Lock()
	// Always kill the old ResultsAll — it caches data at startup and must be
	// restarted to pick up results from a new analysis.
	if appState.resultsProc != nil {
		_ = appState.resultsProc.Kill()
		appState.resultsProc = nil
	}
	// Kill any stale ResultsAll from a previous session that may hold the port.
	if appState.resultsPort != 0 {
		killPort(appState.resultsPort)
	}

	if err != nil {
		port := appState.resultsPort
		if port == 0 {
			port = resultsAllPort
		}
		appState.mu.Unlock()
		w.Header().Set(contentTypeHeader, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"url":  fmt.Sprintf("http://localhost:%d", port),
			"note": "ResultsAll binary not found",
		})
		return
	}

	// Pick a free port — prefer the configured default, fall back to any free one.
	port := findFreePort(resultsAllPort)
	appState.resultsPort = port

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GOLC_RESULTS_PORT=%d", port))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if cmd.Start() == nil {
		appState.resultsProc = cmd.Process
	}
	appState.mu.Unlock()

	// Give ResultsAll a moment to bind the port.
	time.Sleep(800 * time.Millisecond)

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	_ = json.NewEncoder(w).Encode(map[string]string{"url": fmt.Sprintf("http://localhost:%d", port)})
}

var staticHandler = func() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("embedded dist/ not found: " + err.Error())
	}
	return http.StripPrefix("/dist/", http.FileServer(http.FS(sub)))
}()

func handleStatic(w http.ResponseWriter, r *http.Request) {
	staticHandler.ServeHTTP(w, r)
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	// When invoked as an internal analysis subprocess, run the GoLC engine and exit.
	if len(os.Args) > 1 && os.Args[1] == "--internal-run" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: webui --internal-run <platform>")
			os.Exit(1)
		}
		runGolcInProcess(os.Args[2])
		os.Exit(0)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/config", handleGetConfig)
	mux.HandleFunc("/api/run", handleRun)
	mux.HandleFunc("/api/stop", handleStop)
	mux.HandleFunc("/api/events", handleEvents)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/open-results", handleOpenResults)
	mux.HandleFunc("/dist/", handleStatic)

	port := findFreePort(webuiPort)
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("GoLC Web UI started on http://localhost:%d\n", port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}

// ─── HTML Template ────────────────────────────────────────────────────────────

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>GoLC – Line Counter</title>
<link rel="stylesheet" href="/dist/vendors/fontawesome/css/all.min.css">
<link rel="stylesheet" href="/dist/css/theme.min.css">
<style>
  :root {
    --golc-blue: #0d6efd;
    --golc-dark: #0a0e1a;
    --glass-bg: rgba(255,255,255,0.06);
    --glass-border: rgba(255,255,255,0.12);
  }
  body { background: var(--golc-dark); color: #e2e8f0; font-family: 'Segoe UI', sans-serif; min-height: 100vh; }
  .navbar-brand img { height: 36px; }
  .step-indicator { display:flex; align-items:center; gap:0; margin-bottom:2rem; }
  .step-indicator .step { display:flex; align-items:center; gap:.5rem; padding:.4rem 1rem; border-radius:999px;
    font-size:.85rem; font-weight:600; color:#64748b; background:rgba(255,255,255,.04); border:1px solid rgba(255,255,255,.08); }
  .step-indicator .step.active { background:rgba(13,110,253,.18); border-color:rgba(13,110,253,.4); color:#93c5fd; }
  .step-indicator .step.done  { background:rgba(34,197,94,.12); border-color:rgba(34,197,94,.3); color:#86efac; }
  .step-indicator .sep { flex:1; height:1px; background:rgba(255,255,255,.1); min-width:20px; max-width:60px; }
  .platform-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(160px,1fr)); gap:1rem; }
  .platform-card { background:var(--glass-bg); border:1px solid var(--glass-border); border-radius:12px;
    padding:1.25rem 1rem; text-align:center; cursor:pointer; transition:all .2s; }
  .platform-card:hover { border-color:rgba(13,110,253,.5); background:rgba(13,110,253,.1); transform:translateY(-2px); }
  .platform-card.selected { border-color:#0d6efd; background:rgba(13,110,253,.18); }
  .platform-card .icon { font-size:2rem; margin-bottom:.5rem; }
  .platform-card .name { font-size:.82rem; font-weight:600; color:#cbd5e1; }
  .glass-card { background:var(--glass-bg); border:1px solid var(--glass-border); border-radius:16px; padding:1.75rem; }
  .form-label { font-size:.82rem; color:#94a3b8; margin-bottom:.3rem; }
  .form-control, .form-select {
    background:rgba(255,255,255,.06) !important; border:1px solid rgba(255,255,255,.14) !important;
    color:#e2e8f0 !important; border-radius:8px; font-size:.9rem;
  }
  .form-control:focus, .form-select:focus { border-color:#0d6efd !important; box-shadow:0 0 0 3px rgba(13,110,253,.25) !important; }
  .form-control::placeholder { color:#475569 !important; }
  .form-check-input { background-color:rgba(255,255,255,.1); border-color:rgba(255,255,255,.2); }
  .form-check-input:checked { background-color:#0d6efd; border-color:#0d6efd; }
  .btn-primary { background:linear-gradient(135deg,#0d6efd,#6366f1); border:none; border-radius:8px; font-weight:600; color:#fff !important; }
  .btn-primary:hover { background:linear-gradient(135deg,#0b5ed7,#4f46e5); color:#fff !important; }
  .btn-stop { background:linear-gradient(135deg,#dc2626,#b91c1c); border:none; border-radius:8px; font-weight:600; color:#fff !important; }
  .btn-stop:hover { background:linear-gradient(135deg,#b91c1c,#991b1b); color:#fff !important; }
  .btn-success-view { background:linear-gradient(135deg,#059669,#0d6efd); border:none; border-radius:8px;
    font-weight:700; font-size:1.05rem; padding:.65rem 2rem; color:#fff; }
  .btn-success-view:hover { background:linear-gradient(135deg,#047857,#0b5ed7); color:#fff; }
  .progress { height:10px; border-radius:99px; background:rgba(255,255,255,.08); }
  .progress-bar { border-radius:99px; background:linear-gradient(90deg,#0d6efd,#6366f1); transition:width .4s ease; }
  .log-terminal { background:#0a0e1a; border:1px solid rgba(255,255,255,.1); border-radius:10px;
    padding:1rem; height:420px; overflow-y:auto; font-family:monospace; font-size:.78rem;
    color:#94a3b8; line-height:1.6; }
  .log-terminal .log-line { white-space:pre-wrap; word-break:break-all; }
  .log-terminal .log-line.highlight { color:#93c5fd; }
  .log-terminal .log-line.success  { color:#86efac; }
  .log-terminal .log-line.err      { color:#f87171; }
  .phase-badge { display:inline-flex; align-items:center; gap:.4rem; padding:.3rem .8rem;
    border-radius:999px; font-size:.8rem; font-weight:600; }
  .phase-badge.identifying { background:rgba(234,179,8,.12); color:#fde047; border:1px solid rgba(234,179,8,.3); }
  .phase-badge.analyzing   { background:rgba(59,130,246,.12); color:#93c5fd; border:1px solid rgba(59,130,246,.3); }
  .phase-badge.reporting   { background:rgba(168,85,247,.12); color:#d8b4fe; border:1px solid rgba(168,85,247,.3); }
  .phase-badge.complete    { background:rgba(34,197,94,.12); color:#86efac; border:1px solid rgba(34,197,94,.3); }
  .phase-badge.error       { background:rgba(239,68,68,.12); color:#f87171; border:1px solid rgba(239,68,68,.3); }
  .adv-toggle { background:none; border:none; color:#60a5fa; font-size:.84rem; cursor:pointer; padding:0; }
  .adv-toggle:hover { color:#93c5fd; }
  #step-platform, #step-config, #step-run { display:none; }
  #step-platform.active, #step-config.active, #step-run.active { display:block; }
  .result-banner { text-align:center; padding:2.5rem 1rem; }
  .result-banner .big-icon { font-size:3.5rem; margin-bottom:1rem; }
  .navbar { background:rgba(10,14,26,.85) !important; backdrop-filter:blur(12px);
    border-bottom:1px solid rgba(255,255,255,.08); }
</style>
</head>
<body>

<nav class="navbar navbar-dark px-4 py-2">
  <a class="navbar-brand" href="#">
    <img src="/dist/img/Logo.png" alt="GoLC">
  </a>
</nav>

<div class="container py-4" style="max-width:860px;">

  <!-- Step indicator -->
  <div class="step-indicator" id="stepIndicator">
    <div class="step active" id="si1"><i class="fas fa-server fa-sm"></i> Platform</div>
    <div class="sep"></div>
    <div class="step" id="si2"><i class="fas fa-sliders-h fa-sm"></i> Configure</div>
    <div class="sep"></div>
    <div class="step" id="si3"><i class="fas fa-play fa-sm"></i> Analyze</div>
  </div>

  <!-- ─── Step 1: Platform ─── -->
  <div id="step-platform" class="active">
    <h5 class="mb-3" style="color:#cbd5e1;">Choose your DevOps platform</h5>
    <div class="platform-grid">
      <div class="platform-card" data-platform="Github">
        <div class="icon"><i class="fab fa-github"></i></div>
        <div class="name">GitHub.com<br><small style="color:#64748b;font-weight:400;">Cloud</small></div>
      </div>
      <div class="platform-card" data-platform="GithubEnterprise">
        <div class="icon"><i class="fab fa-github"></i></div>
        <div class="name">GitHub Enterprise<br><small style="color:#64748b;font-weight:400;">On-premises</small></div>
      </div>
      <div class="platform-card" data-platform="Gitlab">
        <div class="icon"><i class="fab fa-gitlab"></i></div>
        <div class="name">GitLab<br><small style="color:#64748b;font-weight:400;">Cloud &amp; On-prem</small></div>
      </div>
      <div class="platform-card" data-platform="BitBucket">
        <div class="icon"><i class="fab fa-bitbucket"></i></div>
        <div class="name">Bitbucket Cloud<br><small style="color:#64748b;font-weight:400;">Cloud</small></div>
      </div>
      <div class="platform-card" data-platform="BitBucketSRV">
        <div class="icon"><i class="fab fa-bitbucket"></i></div>
        <div class="name">Bitbucket DC<br><small style="color:#64748b;font-weight:400;">On-premises</small></div>
      </div>
      <div class="platform-card" data-platform="Azure">
        <div class="icon"><i class="fab fa-microsoft"></i></div>
        <div class="name">Azure DevOps<br><small style="color:#64748b;font-weight:400;">Cloud</small></div>
      </div>
      <div class="platform-card" data-platform="File">
        <div class="icon"><i class="fas fa-folder-open"></i></div>
        <div class="name">File Mode<br><small style="color:#64748b;font-weight:400;">Local directories</small></div>
      </div>
    </div>
  </div>

  <!-- ─── Step 2: Config ─── -->
  <div id="step-config">
    <button class="btn btn-link text-secondary mb-3 ps-0" onclick="goToStep(1)">
      <i class="fas fa-arrow-left me-1"></i> Back
    </button>
    <div class="glass-card">
      <div class="d-flex align-items-center gap-2 mb-4">
        <span id="cfg-icon" style="font-size:1.5rem;"></span>
        <div>
          <div style="font-weight:700;font-size:1.05rem;" id="cfg-title"></div>
          <div style="font-size:.78rem;color:#64748b;" id="cfg-subtitle"></div>
        </div>
      </div>

      <form id="configForm" onsubmit="return false;">
        <!-- Basic fields injected here -->
        <div id="basic-fields"></div>

        <!-- Advanced toggle -->
        <div class="mt-3 mb-2">
          <button type="button" class="adv-toggle" onclick="toggleAdvanced()">
            <i class="fas fa-chevron-right me-1" id="adv-chevron"></i> Show advanced options
          </button>
        </div>
        <div id="advanced-fields" style="display:none;">
          <div class="row g-3 mt-1 pt-2" style="border-top:1px solid rgba(255,255,255,.08);">
            <div class="col-md-6" id="adv-defaultBranch-wrap">
              <div class="form-check form-switch mt-2">
                <input class="form-check-input" type="checkbox" id="adv-defaultBranch" checked>
                <label class="form-check-label form-label mb-0" for="adv-defaultBranch">Analyze default branch only</label>
              </div>
            </div>
            <div class="col-md-6">
              <label class="form-label" for="adv-branch">Specific branch name</label>
              <input class="form-control" id="adv-branch" placeholder="e.g. main (leave blank for auto)">
            </div>
            <div class="col-md-6" id="adv-mt-wrap">
              <div class="form-check form-switch mt-2">
                <input class="form-check-input" type="checkbox" id="adv-multithreading" checked>
                <label class="form-check-label form-label mb-0" for="adv-multithreading">Enable multithreading</label>
              </div>
            </div>
            <div class="col-md-6" id="adv-workers-wrap">
              <label class="form-label" for="adv-workers">Number of workers</label>
              <input class="form-control" id="adv-workers" type="number" min="1" max="50" value="10">
            </div>
            <div class="col-md-6">
              <label class="form-label" for="adv-fileExclusion">Exclusion file name</label>
              <input class="form-control" id="adv-fileExclusion" placeholder=".cloc_github_ignore">
            </div>
            <div class="col-md-6">
              <label class="form-label" for="adv-extExclusion">Exclude extensions <small class="text-muted">(comma-separated)</small></label>
              <input class="form-control" id="adv-extExclusion" placeholder=".css,.js">
            </div>
            <!-- Code exclusion presets -->
            <div class="col-12">
              <label class="form-label d-block mb-2">Exclude from analysis</label>
              <div class="d-flex flex-wrap gap-3">
                <div class="form-check">
                  <input class="form-check-input" type="checkbox" id="adv-excludeTests" onchange="syncPresetPaths()">
                  <label class="form-check-label form-label mb-0" for="adv-excludeTests">
                    <i class="fas fa-vial me-1" style="color:#60a5fa;"></i>Test directories
                    <small class="text-muted d-block" style="font-size:.72rem;">test, tests, __tests__, spec, specs, e2e, testdata, fixtures, mocks</small>
                  </label>
                </div>
                <div class="form-check">
                  <input class="form-check-input" type="checkbox" id="adv-excludeVendor" onchange="syncPresetPaths()">
                  <label class="form-check-label form-label mb-0" for="adv-excludeVendor">
                    <i class="fas fa-box me-1" style="color:#a78bfa;"></i>Vendor &amp; modules
                    <small class="text-muted d-block" style="font-size:.72rem;">vendor, node_modules, bower_components, third_party, external</small>
                  </label>
                </div>
              </div>
            </div>
            <div class="col-12">
              <label class="form-label" for="adv-excludePaths">Additional exclude paths <small class="text-muted">(comma-separated, relative to repo root)</small></label>
              <input class="form-control" id="adv-excludePaths" placeholder="pkg/generated,internal/legacy">
              <div class="mt-2 px-3 py-2" style="background:rgba(96,165,250,.07);border-left:3px solid rgba(96,165,250,.4);border-radius:0 4px 4px 0;font-size:.75rem;color:#94a3b8;line-height:1.5;">
                <i class="fas fa-info-circle me-1" style="color:#60a5fa;"></i>
                <strong style="color:#cbd5e1;">How exclusions work:</strong>
                Paths are matched against the <strong style="color:#cbd5e1;">root of each cloned repository</strong> using glob patterns.
                Once matched, exclusion is <strong style="color:#cbd5e1;">fully recursive</strong> — all files and subdirectories underneath are skipped.
                For example, <code style="color:#93c5fd;">test</code> excludes <code style="color:#93c5fd;">/test/**</code> at the root, but not a nested <code style="color:#93c5fd;">src/test</code> directory.
                To exclude a nested path, specify it explicitly: <code style="color:#93c5fd;">src/test</code>.
              </div>
            </div>
            <div class="col-md-6" id="adv-repos-wrap">
              <label class="form-label" for="adv-repos">Specific repositories <small class="text-muted">(comma-separated slugs)</small></label>
              <input class="form-control" id="adv-repos" placeholder="repo-slug1,repo-slug2">
            </div>
            <div class="col-md-6" id="adv-project-wrap">
              <label class="form-label" for="adv-project">Specific project key</label>
              <input class="form-control" id="adv-project" placeholder="PROJECT_KEY">
            </div>
            <div class="col-md-6">
              <div class="form-check form-switch mt-2">
                <input class="form-check-input" type="checkbox" id="adv-resultByFile">
                <label class="form-check-label form-label mb-0" for="adv-resultByFile">Results by file</label>
              </div>
            </div>
          </div>
        </div>

        <div class="d-flex justify-content-end mt-4">
          <button type="button" class="btn btn-primary px-4" onclick="startAnalysis()">
            <i class="fas fa-play me-2"></i>Run Analysis
          </button>
        </div>
      </form>
    </div>
  </div>

  <!-- ─── Step 3: Run / Results ─── -->
  <div id="step-run">
    <div class="glass-card">
      <!-- Running state -->
      <div id="run-running">
        <div class="d-flex align-items-center justify-content-between mb-3">
          <div class="d-flex align-items-center gap-2">
            <span id="run-icon" style="font-size:1.3rem;"></span>
            <span style="font-weight:700;" id="run-platform-name"></span>
          </div>
          <div class="d-flex align-items-center gap-2">
            <span class="phase-badge identifying" id="phase-badge">
              <i class="fas fa-spinner fa-spin fa-sm"></i> Starting...
            </span>
            <button id="stop-btn" class="btn btn-stop btn-sm px-3" onclick="stopAnalysis()" style="display:none;">
              <i class="fas fa-stop me-1"></i>Stop
            </button>
          </div>
        </div>

        <div class="mb-1 d-flex justify-content-between align-items-center">
          <small id="progress-label" style="color:#94a3b8;">Preparing...</small>
          <small id="progress-pct" style="color:#60a5fa;font-weight:700;">0%</small>
        </div>
        <div class="progress mb-3">
          <div class="progress-bar" id="progressBar" style="width:0%"></div>
        </div>

        <div class="log-terminal" id="logTerminal"></div>

        <div id="run-complete-actions" style="display:none;" class="mt-4 result-banner">
          <div class="big-icon" id="result-icon">✅</div>
          <h5 id="result-title" style="color:#86efac;">Analysis Complete!</h5>
          <p id="result-msg" style="color:#94a3b8;font-size:.9rem;"></p>
          <div class="d-flex gap-3 justify-content-center mt-3">
            <button class="btn btn-success-view" onclick="viewResults()">
              <i class="fas fa-chart-bar me-2"></i>View Results
            </button>
            <button class="btn btn-outline-secondary" onclick="goToStep(1)">
              <i class="fas fa-redo me-2"></i>New Analysis
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>

</div><!-- /container -->

<script src="/dist/vendors/bootstrap/js/bootstrap.bundle.min.js"></script>
<script>
const platforms = {
  Github:           { icon:'fab fa-github',     label:'GitHub.com',        sub:'Cloud' },
  GithubEnterprise: { icon:'fab fa-github',     label:'GitHub Enterprise', sub:'On-premises' },
  Gitlab:           { icon:'fab fa-gitlab',     label:'GitLab',            sub:'Cloud & On-premises' },
  BitBucket:        { icon:'fab fa-bitbucket',  label:'Bitbucket Cloud',   sub:'Cloud' },
  BitBucketSRV:     { icon:'fab fa-bitbucket',  label:'Bitbucket DC',      sub:'On-premises' },
  Azure:            { icon:'fab fa-microsoft',  label:'Azure DevOps',      sub:'Cloud' },
  File:             { icon:'fas fa-folder-open',label:'File Mode',         sub:'Local directories' },
};

const TOKEN_PH = '••••••••••••••••••••';
const basicFields = {
  Github:           [{id:'Users',label:'Username / Login',ph:'your-github-login'},
                     {id:'AccessToken',label:'Access Token <small class="text-muted">— Classic PAT: <strong>repo</strong> &nbsp;|&nbsp; Fine-grained: <strong>Contents: Read</strong> &amp; <strong>Metadata: Read</strong></small>',ph:TOKEN_PH,secret:true,html:true},
                     {id:'Organization',label:'Organization',ph:'your-org'}],
  GithubEnterprise: [{id:'Users',label:'Username / Login',ph:'your-login'},
                     {id:'AccessToken',label:'Access Token <small class="text-muted">— Classic PAT: <strong>repo</strong> &nbsp;|&nbsp; Fine-grained: <strong>Contents: Read</strong> &amp; <strong>Metadata: Read</strong></small>',ph:TOKEN_PH,secret:true,html:true},
                     {id:'Organization',label:'Organization',ph:'your-org'},
                     {id:'Url',label:'Server URL',ph:'https://github.yourcompany.com/',onchange:'syncGHEBaseapi()'}],
  Gitlab:           [{id:'Users',label:'Username / Login',ph:'your-gitlab-login'},
                     {id:'AccessToken',label:'Access Token <small class="text-muted">— requires <strong>read_api</strong> &amp; <strong>read_repository</strong> scopes</small>',ph:TOKEN_PH,secret:true,html:true},
                     {id:'Organization',label:'Group URL slug(s) <small class="text-muted">(comma-separated — leave blank to auto-discover all your accessible groups)</small>',ph:'url-slug-1,url-slug-2 (or blank to auto-discover)',html:true},
                     {id:'Url',label:'Server URL <small class="text-muted">(GitLab Cloud — change for on-prem)</small>',ph:'https://gitlab.com/',defaultValue:'https://gitlab.com/',onchange:'syncGitlabProtocol()',html:true}],
  BitBucket:        [{id:'Users',label:'Email address',ph:'you@example.com'},
                     {id:'AccessToken',label:'API Token <small class="text-muted">— requires <strong>Repositories: Read</strong> &amp; <strong>Projects: Read</strong></small>',ph:TOKEN_PH,secret:true,html:true},
                     {id:'Workspace',label:'Workspace slug',ph:'my-workspace'}],
  BitBucketSRV:     [{id:'Users',label:'Username / Login',ph:'your-login'},
                     {id:'AccessToken',label:'Access Token <small class="text-muted">— requires <strong>Project: Read</strong> &amp; <strong>Repository: Read</strong></small>',ph:TOKEN_PH,secret:true,html:true},
                     {id:'Organization',label:'Organization',ph:'your-org'},
                     {id:'Url',label:'Server URL',ph:'https://bitbucket.yourcompany.com/'},
                     {id:'Protocol',label:'Protocol',ph:'https'}],
  Azure:            [{id:'AccessToken',label:'Personal Access Token <small class="text-muted">— requires <strong>Code: Read</strong> &amp; <strong>Project and Team: Read</strong></small>',ph:TOKEN_PH,secret:true,html:true},
                     {id:'Organization',label:'Organization',ph:'your-org'}],
  File:             [{id:'Organization',label:'Organization / Label',ph:'my-org'}],
};

// platforms that support Project & Repos fields in advanced
const supportsProject = new Set(['BitBucket','BitBucketSRV','Azure']);

let currentPlatform = null;
let currentStep = 1;
let eventSource = null;

// ─── Step navigation ───────────────────────────────────────────────────────
function goToStep(n) {
  ['step-platform','step-config','step-run'].forEach((id,i) => {
    const el = document.getElementById(id);
    el.classList.toggle('active', i+1 === n);
  });
  ['si1','si2','si3'].forEach((id,i) => {
    const el = document.getElementById(id);
    el.classList.remove('active','done');
    if (i+1 === n) el.classList.add('active');
    else if (i+1 < n) el.classList.add('done');
  });
  currentStep = n;
}

// ─── Platform selection ────────────────────────────────────────────────────
document.querySelectorAll('.platform-card').forEach(card => {
  card.addEventListener('click', () => {
    const key = card.dataset.platform;
    selectPlatform(key);
  });
});

async function selectPlatform(key) {
  currentPlatform = key;
  document.querySelectorAll('.platform-card').forEach(c =>
    c.classList.toggle('selected', c.dataset.platform === key));

  const p = platforms[key];
  document.getElementById('cfg-icon').innerHTML = '<i class="'+p.icon+'"></i>';
  document.getElementById('cfg-title').textContent = p.label;
  document.getElementById('cfg-subtitle').textContent = p.sub;

  // Load saved config
  let saved = {};
  try {
    const res = await fetch('/api/config?platform='+key);
    saved = await res.json();
  } catch(e) {}

  buildBasicFields(key, saved);
  populateAdvanced(key, saved);

  // Show/hide project field
  document.getElementById('adv-project-wrap').style.display =
    supportsProject.has(key) ? '' : 'none';
  // Hide MT fields for File mode
  const isFile = key === 'File';
  document.getElementById('adv-mt-wrap').style.display = isFile ? 'none' : '';
  document.getElementById('adv-workers-wrap').style.display = isFile ? 'none' : '';
  document.getElementById('adv-defaultBranch-wrap').style.display = isFile ? 'none' : '';
  document.getElementById('adv-repos-wrap').style.display = isFile ? 'none' : '';

  goToStep(2);
}

function buildBasicFields(key, saved) {
  const fields = basicFields[key] || [];
  const container = document.getElementById('basic-fields');
  container.innerHTML = '';
  const row = document.createElement('div');
  row.className = 'row g-3';
  fields.forEach(f => {
    const col = document.createElement('div');
    col.className = 'col-md-6';
    const val = saved[f.id] || f.defaultValue || '';
    const type = f.secret ? 'password' : 'text';
    const input = document.createElement('input');
    input.className = 'form-control';
    input.id = 'f-' + f.id;
    input.type = type;
    input.placeholder = f.ph;
    if (f.secret) {
      input.autocomplete = 'off';
      input.setAttribute('data-lpignore', 'true');
      input.setAttribute('data-1p-ignore', '');
    }
    if (val) input.value = val;
    // For secret fields: clicking when pre-filled clears the value so the
    // user knows they need to re-enter the token (placeholder re-appears).
    if (f.secret && val) {
      input.dataset.prefilled = '1';
      input.addEventListener('focus', function() {
        if (this.dataset.prefilled) {
          this.value = '';
          delete this.dataset.prefilled;
        }
      }, {once: true});
    }
    if (f.onchange) input.addEventListener('input', window[f.onchange] || Function());
    const label = document.createElement('label');
    label.className = 'form-label';
    label.htmlFor = input.id;
    if (f.html) label.innerHTML = f.label; else label.textContent = f.label;
    col.appendChild(label);
    col.appendChild(input);
    row.appendChild(col);
  });
  container.appendChild(row);

  // File mode: append multi-directory widget after the standard fields
  if (key === 'File') {
    buildFileDirectories(saved);
  }
}

function buildFileDirectories(saved) {
  const container = document.getElementById('basic-fields');
  const wrap = document.createElement('div');
  wrap.id = 'file-dirs-wrap';
  wrap.className = 'mt-3';

  const labelRow = document.createElement('div');
  labelRow.className = 'd-flex align-items-center gap-2 mb-1';
  labelRow.innerHTML = '<label class="form-label mb-0" style="font-weight:600;">Directories to analyze</label>'
    + '<button type="button" class="btn btn-sm" id="file-dirs-add"'
    + ' style="background:rgba(99,102,241,.18);color:#a5b4fc;border:1px solid rgba(99,102,241,.35);padding:1px 10px;font-size:.8rem;">'
    + '<i class="fas fa-plus me-1"></i>Add directory</button>';
  wrap.appendChild(labelRow);

  const hint = document.createElement('div');
  hint.className = 'text-muted mb-2';
  hint.style.fontSize = '.78rem';
  hint.innerHTML = 'Enter the <strong>absolute or relative path</strong> to each local directory you want to count. '
    + 'Use <code>.</code> for the current folder. '
    + 'Examples: <code>/home/user/repos/project</code> (Linux/Mac) &nbsp;|&nbsp; <code>C:\\Users\\user\\repos\\project</code> (Windows). '
    + 'Add multiple directories by clicking <em>Add directory</em> — each will be counted and reported separately.';
  wrap.appendChild(hint);

  // Scan subdirectories toggle
  const subDirToggle = document.createElement('div');
  subDirToggle.className = 'form-check form-switch mb-3';
  const scanSaved = !(saved && saved.ScanSubDirs === false);
  subDirToggle.innerHTML = '<input class="form-check-input" type="checkbox" id="file-scanSubDirs"'
    + (scanSaved ? ' checked' : '') + '>'
    + '<label class="form-check-label form-label mb-0" for="file-scanSubDirs">'
    + 'Scan immediate subdirectories as separate repos'
    + ' <small class="text-muted">— enable when the path is a parent folder containing multiple projects (e.g. <code>~/repos</code>); '
    + 'disable when the path itself is the project</small></label>';
  wrap.appendChild(subDirToggle);

  const list = document.createElement('div');
  list.id = 'file-dirs-list';
  wrap.appendChild(list);

  container.appendChild(wrap);

  // Restore saved directories (stored as \n-separated string in Directory field)
  const savedDirs = (saved && saved.Directory) ? saved.Directory.split('\n').filter(Boolean) : [];
  const initial = savedDirs.length ? savedDirs : [''];
  initial.forEach(val => addDirRow(val));

  document.getElementById('file-dirs-add').addEventListener('click', () => addDirRow(''));
}

function addDirRow(val) {
  const list = document.getElementById('file-dirs-list');
  const idx = list.children.length;
  const row = document.createElement('div');
  row.className = 'd-flex gap-2 mb-2 align-items-center file-dir-row';

  const input = document.createElement('input');
  input.className = 'form-control';
  input.type = 'text';
  input.placeholder = idx === 0 ? '/path/to/project  or  C:\\path\\to\\project' : 'Path to another directory...';
  input.dataset.dirIdx = idx;
  if (val) input.value = val;

  const removeBtn = document.createElement('button');
  removeBtn.type = 'button';
  removeBtn.className = 'btn btn-sm';
  removeBtn.style.cssText = 'background:rgba(239,68,68,.15);color:#f87171;border:1px solid rgba(239,68,68,.3);flex-shrink:0;padding:4px 10px;';
  removeBtn.innerHTML = '<i class="fas fa-minus"></i>';
  removeBtn.title = 'Remove this directory';
  removeBtn.addEventListener('click', () => {
    if (document.querySelectorAll('.file-dir-row').length > 1) {
      row.remove();
    }
  });

  row.appendChild(input);
  row.appendChild(removeBtn);
  list.appendChild(row);
}

const PRESET_TEST_PATHS   = ['test','tests','__tests__','spec','specs','e2e','testdata','fixtures','mocks','__mocks__','integration'];
const PRESET_VENDOR_PATHS = ['vendor','node_modules','bower_components','third_party','external'];

function syncPresetPaths() {
  const excludeTests  = document.getElementById('adv-excludeTests').checked;
  const excludeVendor = document.getElementById('adv-excludeVendor').checked;
  const manualRaw = document.getElementById('adv-excludePaths').value;
  // Strip preset paths from the manual field so they don't accumulate
  const allPresets = new Set([...PRESET_TEST_PATHS, ...PRESET_VENDOR_PATHS]);
  const manual = manualRaw.split(',').map(s=>s.trim()).filter(s=>s && !allPresets.has(s));
  const active = [
    ...(excludeTests  ? PRESET_TEST_PATHS  : []),
    ...(excludeVendor ? PRESET_VENDOR_PATHS : []),
    ...manual,
  ];
  document.getElementById('adv-excludePaths').value = active.join(',');
}

function populateAdvanced(key, saved) {
  const defaults = {DefaultBranch:true,Branch:'',Multithreading:true,Workers:10,
    FileExclusion:'',ExtExclusion:[],ExcludePaths:[],ExcludeTests:false,ExcludeVendor:false,
    Repos:'',Project:'',ResultByFile:true};
  const cfg = Object.assign({}, defaults, saved);

  document.getElementById('adv-defaultBranch').checked = !!cfg.DefaultBranch;
  document.getElementById('adv-branch').value = cfg.Branch || '';
  document.getElementById('adv-multithreading').checked = cfg.Multithreading !== false;
  document.getElementById('adv-workers').value = cfg.Workers || 10;
  document.getElementById('adv-fileExclusion').value = cfg.FileExclusion || '';
  document.getElementById('adv-extExclusion').value = Array.isArray(cfg.ExtExclusion)
    ? cfg.ExtExclusion.filter(Boolean).join(',') : (cfg.ExtExclusion||'');
  document.getElementById('adv-excludeTests').checked  = !!cfg.ExcludeTests;
  document.getElementById('adv-excludeVendor').checked = !!cfg.ExcludeVendor;
  // Show additional paths (exclude preset paths — they're represented by the checkboxes)
  const allPresets = new Set([...PRESET_TEST_PATHS, ...PRESET_VENDOR_PATHS]);
  const storedPaths = Array.isArray(cfg.ExcludePaths) ? cfg.ExcludePaths : [];
  const manualOnly = storedPaths.filter(p => p && !allPresets.has(p));
  document.getElementById('adv-excludePaths').value = manualOnly.join(',');
  document.getElementById('adv-repos').value = cfg.Repos || '';
  document.getElementById('adv-project').value = cfg.Project || '';
  document.getElementById('adv-resultByFile').checked = !!cfg.ResultByFile;
}

function toggleAdvanced() {
  const panel = document.getElementById('advanced-fields');
  const chevron = document.getElementById('adv-chevron');
  const btn = chevron.closest('button');
  const open = panel.style.display === '';
  panel.style.display = open ? 'none' : '';
  chevron.className = open ? 'fas fa-chevron-right me-1' : 'fas fa-chevron-down me-1';
  btn.textContent = '';
  btn.appendChild(chevron);
  btn.append(open ? ' Show advanced options' : ' Hide advanced options');
}

// Derive Protocol from the GitLab Url field (only when a custom URL is provided).
function syncGitlabProtocol() {
  const urlEl = document.getElementById('f-Url');
  if (!urlEl) return;
  const raw = urlEl.value.trim();
  if (!raw) return;
  const protoMatch = raw.match(/^(https?):\/\//i);
  const protocol = protoMatch ? protoMatch[1].toLowerCase() : 'https';
  const setHidden = (id, val) => {
    let el = document.getElementById(id);
    if (!el) { el = document.createElement('input'); el.type = 'hidden'; el.id = id; document.body.appendChild(el); }
    el.value = val;
  };
  setHidden('f-Protocol', protocol);
}

// Derive Baseapi (hostname) and Protocol from the GithubEnterprise Url field.
function syncGHEBaseapi() {
  const urlEl = document.getElementById('f-Url');
  if (!urlEl) return;
  const raw = urlEl.value.trim();
  const protoMatch = raw.match(/^(https?):\/\//i);
  const protocol = protoMatch ? protoMatch[1].toLowerCase() : 'https';
  const host = raw.replace(/^https?:\/\//i, '').replace(/\/.*$/, '');
  const setHidden = (id, val) => {
    let el = document.getElementById(id);
    if (!el) { el = document.createElement('input'); el.type = 'hidden'; el.id = id; document.body.appendChild(el); }
    el.value = val;
  };
  setHidden('f-Baseapi', host);
  setHidden('f-Protocol', protocol);
}

function gatherConfig() {
  const fields = basicFields[currentPlatform] || [];
  const cfg = {};
  fields.forEach(f => {
    cfg[f.id] = document.getElementById('f-'+f.id) ? document.getElementById('f-'+f.id).value.trim() : '';
  });
  // File mode: collect multi-directory inputs and scan-subdirs toggle
  if (currentPlatform === 'File') {
    const dirs = Array.from(document.querySelectorAll('.file-dir-row input'))
      .map(el => el.value.trim()).filter(Boolean);
    cfg.Directory = dirs.join('\n');
    const scanEl = document.getElementById('file-scanSubDirs');
    cfg.ScanSubDirs = scanEl ? scanEl.checked : true;
  }
  // Bitbucket Cloud: Organization = Workspace (same value, one field removed from UI)
  if (currentPlatform === 'BitBucket') {
    cfg.Organization = cfg.Workspace;
  }
  // Auto-derive Baseapi and Protocol for GitHub Enterprise from the Url field
  if (currentPlatform === 'GithubEnterprise') {
    syncGHEBaseapi();
    cfg.Baseapi   = (document.getElementById('f-Baseapi')   || {}).value || '';
    cfg.Protocol  = (document.getElementById('f-Protocol')  || {}).value || 'https';
  }
  // Auto-derive Protocol for GitLab from the Url field (only when a custom URL is set)
  if (currentPlatform === 'Gitlab' && cfg.Url) {
    syncGitlabProtocol();
    const derived = (document.getElementById('f-Protocol') || {}).value;
    if (derived) cfg.Protocol = derived;
  }
  cfg.DefaultBranch = document.getElementById('adv-defaultBranch').checked;
  cfg.Branch = document.getElementById('adv-branch').value.trim();
  cfg.Multithreading = document.getElementById('adv-multithreading').checked;
  cfg.Workers = parseInt(document.getElementById('adv-workers').value) || 10;
  cfg.NumberWorkerRepos = cfg.Workers;
  cfg.FileExclusion = document.getElementById('adv-fileExclusion').value.trim();
  const ext = document.getElementById('adv-extExclusion').value.trim();
  cfg.ExtExclusion = ext ? ext.split(',').map(s=>s.trim()).filter(Boolean) : [];
  cfg.ExcludeTests  = document.getElementById('adv-excludeTests').checked;
  cfg.ExcludeVendor = document.getElementById('adv-excludeVendor').checked;
  const manualPaths = document.getElementById('adv-excludePaths').value.trim();
  const manualArr   = manualPaths ? manualPaths.split(',').map(s=>s.trim()).filter(Boolean) : [];
  const presetArr   = [
    ...(cfg.ExcludeTests  ? PRESET_TEST_PATHS  : []),
    ...(cfg.ExcludeVendor ? PRESET_VENDOR_PATHS : []),
  ];
  cfg.ExcludePaths  = [...new Set([...presetArr, ...manualArr])];
  cfg.Repos = document.getElementById('adv-repos').value.trim();
  cfg.Project = document.getElementById('adv-project').value.trim();
  cfg.ResultByFile = document.getElementById('adv-resultByFile').checked;
  cfg.ResultAll = true;
  return cfg;
}

// ─── Run analysis ──────────────────────────────────────────────────────────
async function startAnalysis() {
  const cfg = gatherConfig();
  const p = platforms[currentPlatform];

  // Switch to run step
  document.getElementById('run-icon').innerHTML = '<i class="'+p.icon+'"></i>';
  document.getElementById('run-platform-name').textContent = p.label;
  document.getElementById('run-complete-actions').style.display = 'none';
  document.getElementById('logTerminal').innerHTML = '';
  document.getElementById('stop-btn').style.display = '';
  setPhase('identifying', 0);
  goToStep(3);

  // Start SSE before POST so we don't miss events
  startSSE();

  try {
    const res = await fetch('/api/run', {
      method: 'POST',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify({platform: currentPlatform, config: cfg})
    });
    if (!res.ok) {
      const msg = await res.text();
      appendLog('ERROR: '+msg, 'err');
      setPhase('error', 100);
    }
  } catch(e) {
    appendLog('Failed to start: '+e.message, 'err');
    setPhase('error', 100);
  }
}

function startSSE() {
  if (eventSource) { eventSource.close(); eventSource = null; }
  eventSource = new EventSource('/api/events');
  eventSource.onmessage = function(e) {
    const ev = JSON.parse(e.data);
    handleEvent(ev);
  };
  eventSource.onerror = function() {
    // Will auto-reconnect
  };
}

function handleEvent(ev) {
  if (ev.type === 'log') {
    appendLog(ev.message);
  } else if (ev.type === 'progress') {
    updateProgress(ev);
    appendLog(ev.message, 'highlight');
  } else if (ev.type === 'complete') {
    updateProgress(ev);
    appendLog(ev.message, 'success');
    showComplete(false, ev.message);
    if (eventSource) { eventSource.close(); eventSource = null; }
  } else if (ev.type === 'error') {
    appendLog(ev.message, 'err');
    showComplete(true, ev.message);
    if (eventSource) { eventSource.close(); eventSource = null; }
  }
}

function updateProgress(ev) {
  const pct = ev.pct || 0;
  document.getElementById('progressBar').style.width = pct+'%';
  document.getElementById('progress-pct').textContent = pct+'%';
  // Use label (summary) for the bar; fall back to message when label is absent
  document.getElementById('progress-label').textContent = ev.label || ev.message || '';
  if (ev.phase) setPhase(ev.phase, pct);
}

function setPhase(phase, pct) {
  const badge = document.getElementById('phase-badge');
  const labels = {
    identifying: '<i class="fas fa-spinner fa-spin fa-sm"></i> Identifying repositories',
    analyzing:   '<i class="fas fa-spinner fa-spin fa-sm"></i> Analyzing repositories',
    reporting:   '<i class="fas fa-spinner fa-spin fa-sm"></i> Generating reports',
    complete:    '<i class="fas fa-check-circle fa-sm"></i> Complete',
    error:       '<i class="fas fa-times-circle fa-sm"></i> Error',
    idle:        '<i class="fas fa-circle fa-sm"></i> Idle',
  };
  badge.className = 'phase-badge ' + (phase||'identifying');
  badge.innerHTML = labels[phase] || labels.identifying;
  if (pct !== undefined) {
    document.getElementById('progressBar').style.width = pct+'%';
    document.getElementById('progress-pct').textContent = pct+'%';
  }
}

function appendLog(msg, cls) {
  if (!msg || !msg.trim()) return;
  const term = document.getElementById('logTerminal');
  const line = document.createElement('div');
  line.className = 'log-line' + (cls ? ' '+cls : '');
  line.textContent = msg;
  term.appendChild(line);
  // Cap at 200 nodes to prevent DOM bloat and layout-reflow freezes
  while (term.children.length > 200) {
    term.removeChild(term.firstChild);
  }
  term.scrollTop = term.scrollHeight;
}

async function stopAnalysis() {
  document.getElementById('stop-btn').disabled = true;
  try {
    await fetch('/api/stop', {method:'POST'});
  } catch(e) {}
}

function showComplete(isError, msg) {
  document.getElementById('stop-btn').style.display = 'none';
  const panel = document.getElementById('run-complete-actions');
  const icon = document.getElementById('result-icon');
  const title = document.getElementById('result-title');
  const msgEl = document.getElementById('result-msg');

  if (isError) {
    icon.textContent = '❌';
    title.style.color = '#f87171';
    title.textContent = 'Analysis Failed';
    msgEl.textContent = msg || 'Check the log above for details.';
    setPhase('error', 100);
  } else {
    icon.textContent = '✅';
    title.style.color = '#86efac';
    title.textContent = 'Analysis Complete!';
    msgEl.textContent = 'Results are ready. Click "View Results" to open the dashboard.';
    setPhase('complete', 100);
  }
  panel.style.display = '';
  document.getElementById('progressBar').style.width = '100%';
  document.getElementById('progress-pct').textContent = '100%';
}

// ─── View Results ──────────────────────────────────────────────────────────
async function viewResults() {
  try {
    const res = await fetch('/api/open-results', {method:'POST'});
    const data = await res.json();
    window.open(data.url, '_blank');
  } catch(e) {
    window.open('http://localhost:{{.ResultsAllPort}}', '_blank');
  }
}

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}
</script>
</body>
</html>`
