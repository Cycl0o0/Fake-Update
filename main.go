package main

/*
#include <locale.h>
#include <stdlib.h>
*/
import "C"

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	gc "github.com/rthornton128/goncurses"
)

type Config struct {
	Duration    int
	OSName      string
	OSVersion   string
	DeviceName  string
	BuildID     string
	Hostname    string
	Arch        string
	CPUModel    string
	CPUCores    int
	CPULogical  int
	MemoryGB    int
	RootDiskGB  int
	RootAvailGB int
	RootDevice  string
	PowerState  string
	DistroID    string
	DistroLike  string
	NoAI        bool
}

type LogLine struct {
	text  string
	color int16
}

type Badge struct {
	text  string
	color int16
}

type UpdateProfile struct {
	Channel        string
	TotalBundleGB  float64
	PackageTotal   int
	Headline       string
	Advisory       string
	PayloadLabel   string
	IntegrityLabel string
}

type UpdateMetrics struct {
	SessionID       string
	BundleCurrentGB float64
	PackageCurrent  int
	NetMbps         int
	DiskMBps        int
	WorkerCount     int
	StageProgress   float64
	Integrity       string
	PowerState      string
}

const (
	cDefault    int16 = 1
	cCyan       int16 = 2
	cGreen      int16 = 3
	cYellow     int16 = 4
	cRed        int16 = 5
	cBarFilled  int16 = 6
	cBarEmpty   int16 = 7
	cMagenta    int16 = 8
	cDim        int16 = 9
	cAlertBadge int16 = 10
	cInfoBadge  int16 = 11
	cStageBadge int16 = 12
	cDimCyan    int16 = 13
	cBarHead    int16 = 14
	cDarkGreen  int16 = 15
)

func main() {
	detected := detectSystem()
	cfg := detected

	flag.IntVar(&cfg.Duration, "duration", 0, "Duration in seconds (required)")
	flag.IntVar(&cfg.Duration, "d", 0, "Duration in seconds (shorthand)")
	flag.StringVar(&cfg.OSName, "os", detected.OSName, "OS name")
	flag.StringVar(&cfg.OSVersion, "version", detected.OSVersion, "OS version string")
	flag.StringVar(&cfg.DeviceName, "device", detected.DeviceName, "Device name")
	flag.StringVar(&cfg.BuildID, "build", detected.BuildID, "Build/revision number")

	flag.BoolVar(&cfg.NoAI, "no-ai", false, "Disable AI-generated log lines (uses hardcoded logs)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fake-update - A fake system update screen\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  fake-update -d <seconds> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  fake-update -d 300\n")
		fmt.Fprintf(os.Stderr, "  fake-update -d 600 -os macOS -version \"Sonoma 14.5\"\n")
		fmt.Fprintf(os.Stderr, "  fake-update -d 3600 -os Windows -version \"11 23H2\"\n")
		fmt.Fprintf(os.Stderr, "  fake-update -d 300 -no-ai\n\n")
		fmt.Fprintf(os.Stderr, "AI log generation is enabled by default (requires OPENAI_API_KEY).\n")
		fmt.Fprintf(os.Stderr, "Falls back to built-in logs if API key is missing or connection fails.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nPress 'q' to quit.\n")
	}

	flag.Parse()
	if cfg.Duration <= 0 {
		flag.Usage()
		os.Exit(1)
	}
	rand.Seed(time.Now().UnixNano())

	cLocale := C.CString("")
	C.setlocale(C.LC_ALL, cLocale)
	C.free(unsafe.Pointer(cLocale))

	stdscr, err := gc.Init()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer gc.End()

	gc.StartColor()
	gc.Cursor(0)
	gc.Echo(false)
	gc.CBreak(true)
	stdscr.Keypad(true)
	stdscr.Timeout(50)

	gc.InitPair(cDefault, gc.C_WHITE, gc.C_BLACK)
	gc.InitPair(cCyan, gc.C_CYAN, gc.C_BLACK)
	gc.InitPair(cGreen, gc.C_GREEN, gc.C_BLACK)
	gc.InitPair(cYellow, gc.C_YELLOW, gc.C_BLACK)
	gc.InitPair(cRed, gc.C_RED, gc.C_BLACK)
	gc.InitPair(cBarFilled, gc.C_GREEN, gc.C_BLACK)
	gc.InitPair(cBarEmpty, gc.C_GREEN, gc.C_BLACK)
	gc.InitPair(cMagenta, gc.C_MAGENTA, gc.C_BLACK)
	gc.InitPair(cDim, gc.C_BLUE, gc.C_BLACK)
	gc.InitPair(cAlertBadge, gc.C_WHITE, gc.C_RED)
	gc.InitPair(cInfoBadge, gc.C_BLACK, gc.C_CYAN)
	gc.InitPair(cStageBadge, gc.C_BLACK, gc.C_YELLOW)
	gc.InitPair(cDimCyan, gc.C_CYAN, gc.C_BLACK)
	gc.InitPair(cBarHead, gc.C_BLACK, gc.C_WHITE)
	gc.InitPair(cDarkGreen, gc.C_GREEN, gc.C_BLACK)

	runUpdateScreen(stdscr, cfg)
}

// ====================== DETECT ======================

func detectSystem() Config {
	cfg := Config{
		OSName:     "Linux",
		Arch:       normalizeArch(runtime.GOARCH),
		CPULogical: runtime.NumCPU(),
		PowerState: "AC",
	}
	if host, err := os.Hostname(); err == nil {
		cfg.Hostname = host
	}
	switch runtime.GOOS {
	case "darwin":
		cfg.OSName = "macOS"
		if data, err := os.ReadFile("/System/Library/CoreServices/SystemVersion.plist"); err == nil {
			content := string(data)
			cfg.OSVersion = pluckPlist(content, "ProductUserVisibleVersion")
			cfg.BuildID = pluckPlist(content, "ProductBuildVersion")
		}
		if cfg.OSVersion == "" {
			cfg.OSVersion = strings.TrimSpace(readCommandOutput("sw_vers", "-productVersion"))
		}
		if cfg.BuildID == "" {
			cfg.BuildID = strings.TrimSpace(readCommandOutput("sw_vers", "-buildVersion"))
		}
	case "linux":
		var prettyName string
		if f, err := os.Open("/etc/os-release"); err == nil {
			defer f.Close()
			s := bufio.NewScanner(f)
			for s.Scan() {
				k, v := splitKV(s.Text())
				switch k {
				case "NAME":
					cfg.OSName = v
				case "PRETTY_NAME":
					prettyName = v
				case "VERSION":
					cfg.OSVersion = v
				case "VERSION_ID":
					if cfg.OSVersion == "" {
						cfg.OSVersion = v
					}
				case "ID":
					cfg.DistroID = v
				case "ID_LIKE":
					cfg.DistroLike = v
				case "BUILD_ID":
					cfg.BuildID = v
				}
			}
			if cfg.OSVersion == "" {
				if s := strings.TrimSpace(strings.TrimPrefix(prettyName, cfg.OSName)); s != "" {
					cfg.OSVersion = s
				} else {
					cfg.OSVersion = "Rolling"
				}
			}
		}
		if cfg.BuildID == "" {
			if data, err := os.ReadFile("/proc/version"); err == nil {
				if parts := strings.Fields(string(data)); len(parts) >= 3 {
					cfg.BuildID = parts[2]
				}
			}
		}
	case "windows":
		cfg.OSName, cfg.OSVersion, cfg.BuildID = detectWindowsRelease()
	}

	cfg.CPUModel, cfg.CPUCores, cfg.CPULogical = detectCPU(cfg.CPULogical)
	cfg.MemoryGB = detectMemoryGB()
	cfg.RootDevice, cfg.RootDiskGB, cfg.RootAvailGB = detectRootVolume()
	if power := detectPowerState(); power != "" {
		cfg.PowerState = power
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = readDMI()
	}
	if cfg.DeviceName == "" && runtime.GOOS == "darwin" {
		cfg.DeviceName = strings.TrimSpace(readCommandOutput("sysctl", "-n", "hw.model"))
	}
	if cfg.DeviceName == "" && runtime.GOOS == "windows" {
		if model := strings.TrimSpace(detectWindowsModel()); model != "" {
			cfg.DeviceName = model
		} else if name := strings.TrimSpace(envFirst("COMPUTERNAME", "HOSTNAME")); name != "" {
			cfg.DeviceName = name
		}
	}
	if cfg.OSVersion == "" {
		cfg.OSVersion = "Rolling"
	}
	if cfg.BuildID == "" {
		cfg.BuildID = "unknown"
	}
	return cfg
}

func pluckPlist(content, key string) string {
	idx := strings.Index(content, key)
	if idx == -1 {
		return ""
	}
	rest := content[idx:]
	if s := strings.Index(rest, "<string>"); s != -1 {
		rest = rest[s+8:]
		if e := strings.Index(rest, "</string>"); e != -1 {
			return rest[:e]
		}
	}
	return ""
}

func splitKV(line string) (string, string) {
	if i := strings.IndexByte(line, '='); i >= 0 {
		return line[:i], strings.Trim(line[i+1:], "\"")
	}
	return "", ""
}

func readDMI() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	for _, p := range []struct{ path, rej string }{
		{"/sys/devices/virtual/dmi/id/product_name", "System Product Name"},
		{"/sys/devices/virtual/dmi/id/product_version", "System Version"},
		{"/sys/devices/virtual/dmi/id/board_name", ""},
	} {
		if data, err := os.ReadFile(p.path); err == nil {
			if n := strings.TrimSpace(string(data)); n != "" && n != p.rej {
				return n
			}
		}
	}
	return ""
}

func detectCPU(defaultLogical int) (string, int, int) {
	switch runtime.GOOS {
	case "linux":
		return readLinuxCPUInfo(defaultLogical)
	case "darwin":
		model := strings.TrimSpace(readCommandOutput("sysctl", "-n", "machdep.cpu.brand_string"))
		cores := parseInt(strings.TrimSpace(readCommandOutput("sysctl", "-n", "hw.physicalcpu")))
		logical := parseInt(strings.TrimSpace(readCommandOutput("sysctl", "-n", "hw.logicalcpu")))
		if logical == 0 {
			logical = defaultLogical
		}
		if cores == 0 {
			cores = logical
		}
		return model, cores, logical
	case "windows":
		model, cores, logical := detectWindowsCPU(defaultLogical)
		if model == "" {
			model = strings.TrimSpace(envFirst("PROCESSOR_IDENTIFIER", "PROCESSOR_ARCHITECTURE"))
		}
		if logical == 0 {
			logical = defaultLogical
		}
		if cores == 0 {
			cores = logical
		}
		return model, cores, logical
	default:
		return "", defaultLogical, defaultLogical
	}
}

func readLinuxCPUInfo(defaultLogical int) (string, int, int) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", defaultLogical, defaultLogical
	}

	var model string
	var cores, logical int
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "model name":
			if model == "" {
				model = value
			}
		case "cpu cores":
			if cores == 0 {
				cores = parseInt(value)
			}
		case "processor":
			logical++
		}
	}
	if logical == 0 {
		logical = defaultLogical
	}
	if cores == 0 {
		cores = logical
	}
	return cleanCPUModel(model), cores, logical
}

func detectMemoryGB() int {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "MemTotal:") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0
			}
			kb := parseInt(fields[1])
			if kb == 0 {
				return 0
			}
			gb := int(math.Round(float64(kb) / 1024.0 / 1024.0))
			if gb < 1 {
				gb = 1
			}
			return gb
		}
	case "darwin":
		bytes := parseInt(strings.TrimSpace(readCommandOutput("sysctl", "-n", "hw.memsize")))
		if bytes > 0 {
			gb := int(math.Round(float64(bytes) / (1024.0 * 1024.0 * 1024.0)))
			if gb < 1 {
				gb = 1
			}
			return gb
		}
	case "windows":
		bytes := parseInt(strings.TrimSpace(runPowershell("(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory")))
		if bytes > 0 {
			gb := int(math.Round(float64(bytes) / (1024.0 * 1024.0 * 1024.0)))
			if gb < 1 {
				gb = 1
			}
			return gb
		}
	}
	return 0
}

func detectRootVolume() (string, int, int) {
	path := "/"
	if runtime.GOOS == "windows" {
		path = "C:\\"
	}

	out := strings.TrimSpace(readCommandOutput("df", "-kP", path))
	if runtime.GOOS == "windows" {
		device, totalGB, availGB := detectWindowsRootVolume()
		if totalGB > 0 {
			return device, totalGB, availGB
		}
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return "", 0, 0
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return "", 0, 0
	}
	totalKB := parseInt(fields[1])
	availKB := parseInt(fields[3])
	device := fields[0]
	totalGB := kbToRoundedGB(totalKB)
	availGB := kbToRoundedGB(availKB)
	return device, totalGB, availGB
}

func detectPowerState() string {
	if runtime.GOOS != "linux" {
		switch runtime.GOOS {
		case "darwin":
			power := strings.ToLower(readCommandOutput("pmset", "-g", "batt"))
			switch {
			case strings.Contains(power, "ac power"):
				return "AC"
			case strings.Contains(power, "battery power"):
				return "BAT"
			}
		case "windows":
			if state := strings.TrimSpace(runPowershell(`$b=Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue | Select-Object -First 1 BatteryStatus; if($b){$b} else {0}`)); state != "" {
				switch state {
				case "1":
					return "BAT"
				case "2", "3", "6", "7", "8", "9":
					return "AC"
				}
			}
		}
		return ""
	}
	powerEntries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return ""
	}
	for _, entry := range powerEntries {
		base := "/sys/class/power_supply/" + entry.Name()
		typeData, err := os.ReadFile(base + "/type")
		if err != nil {
			continue
		}
		switch strings.TrimSpace(string(typeData)) {
		case "Mains", "USB", "USB_C":
			state, err := os.ReadFile(base + "/online")
			if err == nil && strings.TrimSpace(string(state)) == "1" {
				return "AC"
			}
		case "Battery":
			status, err := os.ReadFile(base + "/status")
			if err == nil {
				switch strings.ToLower(strings.TrimSpace(string(status))) {
				case "discharging":
					return "BAT"
				case "charging", "full", "not charging":
					return "AC"
				}
			}
		}
	}
	return ""
}

func normalizeArch(arch string) string {
	switch arch {
	case "amd64":
		return "x86_64"
	case "386":
		return "x86"
	default:
		return arch
	}
}

func cleanCPUModel(model string) string {
	replacements := []string{
		"(R)", "",
		"(TM)", "",
		"CPU", "",
		"Processor", "",
		"  ", " ",
	}
	replacer := strings.NewReplacer(replacements...)
	clean := strings.TrimSpace(replacer.Replace(model))
	for strings.Contains(clean, "  ") {
		clean = strings.ReplaceAll(clean, "  ", " ")
	}
	return clean
}

func kbToRoundedGB(kb int) int {
	if kb <= 0 {
		return 0
	}
	gb := int(math.Round(float64(kb) / 1024.0 / 1024.0))
	if gb < 1 {
		gb = 1
	}
	return gb
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func readCommandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func runPowershell(script string) string {
	return readCommandOutput("powershell", "-NoProfile", "-Command", script)
}

func detectWindowsRelease() (string, string, string) {
	product := strings.TrimSpace(runPowershell(`$v=Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'; $v.ProductName`))
	displayVersion := strings.TrimSpace(runPowershell(`$v=Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'; if($v.DisplayVersion){$v.DisplayVersion}elseif($v.ReleaseId){$v.ReleaseId}`))
	build := strings.TrimSpace(runPowershell(`$v=Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'; $v.CurrentBuildNumber`))
	if product == "" {
		product = "Windows"
	}
	name := "Windows"
	version := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(product, "Microsoft"), "Windows"))
	if displayVersion != "" {
		version = strings.TrimSpace(version + " " + displayVersion)
	}
	if version == "" {
		version = strings.TrimSpace(runPowershell(`(Get-CimInstance Win32_OperatingSystem).Version`))
	}
	if build == "" {
		build = strings.TrimSpace(runPowershell(`(Get-CimInstance Win32_OperatingSystem).BuildNumber`))
	}
	return name, version, build
}

func detectWindowsModel() string {
	return strings.TrimSpace(runPowershell(`(Get-CimInstance Win32_ComputerSystem).Model`))
}

func detectWindowsCPU(defaultLogical int) (string, int, int) {
	script := `$cpu=Get-CimInstance Win32_Processor | Select-Object -First 1; if($cpu){"$($cpu.Name)|$($cpu.NumberOfCores)|$($cpu.NumberOfLogicalProcessors)"}`
	fields := strings.Split(strings.TrimSpace(runPowershell(script)), "|")
	if len(fields) < 3 {
		return "", 0, defaultLogical
	}
	return cleanCPUModel(fields[0]), parseInt(fields[1]), parseInt(fields[2])
}

func detectWindowsRootVolume() (string, int, int) {
	script := `$d=Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='C:'" | Select-Object -First 1; if($d){"$($d.DeviceID)|$($d.Size)|$($d.FreeSpace)"}`
	fields := strings.Split(strings.TrimSpace(runPowershell(script)), "|")
	if len(fields) < 3 {
		return "", 0, 0
	}
	totalBytes := parseInt(fields[1])
	freeBytes := parseInt(fields[2])
	return fields[0], bytesToRoundedGB(totalBytes), bytesToRoundedGB(freeBytes)
}

func bytesToRoundedGB(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	gb := int(math.Round(float64(bytes) / (1024.0 * 1024.0 * 1024.0)))
	if gb < 1 {
		gb = 1
	}
	return gb
}

// ====================== RENDER ======================

func runUpdateScreen(stdscr *gc.Window, cfg Config) {
	startTime := time.Now()
	endTime := startTime.Add(time.Duration(cfg.Duration) * time.Second)
	frame := 0

	stages := getStages(cfg)
	profile := getUpdateProfile(cfg)
	sessionID := fmt.Sprintf("%s-%04X", startTime.Format("150405"), rand.Intn(0x10000))
	spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	aiGen := newAILogGenerator(cfg, profile)

	logBuf := make([]LogLine, 0, 64)
	lastLogFrame := 0

	for {
		now := time.Now()
		if now.After(endTime) {
			break
		}

		elapsed := now.Sub(startTime).Seconds()
		total := endTime.Sub(startTime).Seconds()
		progress := elapsed / total
		remaining := endTime.Sub(now)
		stageIdx := int(progress * float64(len(stages)))
		if stageIdx >= len(stages) {
			stageIdx = len(stages) - 1
		}
		metrics := buildUpdateMetrics(cfg, profile, progress, frame, stageIdx, len(stages), sessionID)

		rows, cols := stdscr.MaxYX()
		stdscr.Erase()
		cy := rows / 2
		cx := cols / 2

		// -- OUTER FRAME --
		drawFrame(stdscr, 0, 0, cols, rows, frame, cCyan)

		// -- TOP BAR (row 1) --
		drawTopStatusBar(stdscr, cols, now, cfg, profile, metrics)

		// -- MAIN PANEL --
		panelW := 80
		if panelW > cols-6 {
			panelW = cols - 6
		}
		panelH := rows - 8
		if panelH < 16 {
			panelH = 16
		}
		if panelH > 36 {
			panelH = 36
		}
		px := cx - panelW/2
		py := cy - panelH/2 + 1
		drawFrame(stdscr, px, py, panelW, panelH, frame, cDimCyan)

		// Clear panel interior
		stdscr.ColorOn(cDefault)
		for y := py + 1; y < py+panelH-1; y++ {
			for x := px + 1; x < px+panelW-1; x++ {
				stdscr.MovePrint(y, x, " ")
			}
		}

		// -- PANEL TITLE in border --
		titleTag := fmt.Sprintf(" SYSTEM UPDATE ")
		stdscr.ColorOn(cInfoBadge)
		stdscr.AttrOn(gc.A_BOLD)
		stdscr.MovePrint(py, px+panelW/2-len(titleTag)/2, titleTag)
		stdscr.AttrOff(gc.A_BOLD)

		compact := panelH < 22
		subtitleY := py + 2
		hostLabel := cfg.Hostname
		if hostLabel == "" {
			hostLabel = cfg.DeviceName
		}
		if hostLabel == "" {
			hostLabel = "local-host"
		}
		subtitle := fmt.Sprintf(" %s | %s | build %s ", hostLabel, metrics.SessionID, cfg.BuildID)
		stdscr.ColorOn(cDimCyan)
		printCentered(stdscr, subtitleY, cx, truncateText(subtitle, panelW-4))

		contentY := subtitleY + 2
		if !compact {
			panelBadges := []Badge{{text: "TRACK " + strings.ToUpper(profile.Channel), color: cInfoBadge}}
			if cfg.MemoryGB > 0 {
				panelBadges = append(panelBadges, Badge{text: fmt.Sprintf("%d GB RAM", cfg.MemoryGB), color: cBarFilled})
			}
			if threadBadge := formatThreadBadge(cfg); threadBadge != "" {
				panelBadges = append(panelBadges, Badge{text: threadBadge, color: cStageBadge})
			}
			drawCenteredBadges(stdscr, contentY, cx, panelW-6, panelBadges)
			contentY += 2
		}

		stdscr.ColorOn(cGreen)
		stdscr.AttrOn(gc.A_BOLD)
		printCentered(stdscr, contentY, cx, truncateText(profile.Headline, panelW-8))
		stdscr.AttrOff(gc.A_BOLD)
		contentY++

		stdscr.ColorOn(cYellow)
		printCentered(stdscr, contentY, cx, truncateText(profile.Advisory, panelW-8))
		contentY += 2

		// -- PROGRESS BAR --
		barY := contentY
		barW := panelW - 10
		if barW < 20 {
			barW = 20
		}
		barX := cx - barW/2
		drawProgressBar(stdscr, barX, barY, barW, progress, frame)

		// -- PERCENTAGE --
		pctY := barY + 1
		pctStr := fmt.Sprintf(" %.1f%% ", progress*100)
		stdscr.ColorOn(cGreen)
		stdscr.AttrOn(gc.A_BOLD)
		printCentered(stdscr, pctY, cx, pctStr)
		stdscr.AttrOff(gc.A_BOLD)

		// -- TIME BADGE --
		timeStr := fmt.Sprintf(" ETA %s | Phase %02d/%02d | Stage %.0f%% ", formatRemaining(remaining), stageIdx+1, len(stages), metrics.StageProgress*100)
		stdscr.ColorOn(cInfoBadge)
		printCentered(stdscr, pctY+1, cx, truncateText(timeStr, panelW-8))

		metricY := pctY + 3
		metricLine := fmt.Sprintf("%s %.1f / %.1f GB | Packages %d / %d | Workers %d",
			profile.PayloadLabel, metrics.BundleCurrentGB, profile.TotalBundleGB, metrics.PackageCurrent, profile.PackageTotal, metrics.WorkerCount)
		stdscr.ColorOn(cDefault)
		printCentered(stdscr, metricY, cx, truncateText(metricLine, panelW-8))
		if !compact {
			metricY++
			metricLine = fmt.Sprintf("Integrity %s | Network %d Mb/s | Disk %d MB/s | Power %s",
				metrics.Integrity, metrics.NetMbps, metrics.DiskMBps, metrics.PowerState)
			stdscr.ColorOn(cDimCyan)
			printCentered(stdscr, metricY, cx, truncateText(metricLine, panelW-8))

			metricY++
			metricLine = formatSpecSummary(cfg)
			stdscr.ColorOn(cDefault)
			printCentered(stdscr, metricY, cx, truncateText(metricLine, panelW-8))
		}

		stageY := metricY + 2
		stageLines := 5
		if compact {
			stageLines = 3
		}
		drawnStages := drawStageList(stdscr, stages, stageIdx, stageY, cx, panelW-8, frame, spinChars, stageLines)

		// -- HORIZONTAL RULE --
		ruleY := stageY + drawnStages + 1
		if ruleY < py+panelH-2 {
			stdscr.ColorOn(cDim)
			stdscr.MoveAddChar(ruleY, px, gc.ACS_LTEE)
			for x := px + 1; x < px+panelW-1; x++ {
				stdscr.MoveAddChar(ruleY, x, gc.ACS_HLINE)
			}
			stdscr.MoveAddChar(ruleY, px+panelW-1, gc.ACS_RTEE)
			logTag := " Activity "
			stdscr.ColorOn(cDim)
			stdscr.MovePrint(ruleY, px+3, logTag)
		}

		// -- LOG PANEL --
		logY := ruleY + 1
		logH := (py + panelH - 1) - logY
		if logH < 1 {
			logH = 1
		}
		if frame-lastLogFrame > 4 {
			aiGen.ensureBuffer(stageIdx, len(stages), stages[stageIdx])
			logBuf = appendLogAI(logBuf, aiGen, cfg, profile, metrics, frame, stageIdx, stages[stageIdx])
			lastLogFrame = frame
		}
		drawLog(stdscr, logBuf, px+3, panelW-6, logY, logH)

		// -- PANEL FOOTER NOTE --
		warnBarY := py + panelH
		if warnBarY < rows-2 {
			stdscr.ColorOn(cDimCyan)
			printCentered(stdscr, warnBarY, cx, truncateText(" Automatic restart may occur when integrity verification completes ", panelW-6))
		}

		// -- BOTTOM STATUS BAR (last row inside frame) --
		drawBottomStatusBar(stdscr, rows-2, cols, elapsed, remaining, progress, stageIdx, len(stages), profile.PackageTotal, metrics)

		stdscr.Refresh()
		frame++

		if ch := stdscr.GetChar(); ch == 'q' || ch == 'Q' {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	showComplete(stdscr, cfg)
}

// ====================== DRAW HELPERS ======================

func printCentered(w *gc.Window, y, cx int, s string) {
	x := cx - len(s)/2
	if x < 0 {
		x = 0
	}
	w.MovePrint(y, x, s)
}

// drawFrame draws an ACS box with a traveling highlight spark on the border.
func drawFrame(w *gc.Window, x, y, width, height, frame int, baseColor int16) {
	w.ColorOn(baseColor)
	w.MoveAddChar(y, x, gc.ACS_ULCORNER)
	w.MoveAddChar(y, x+width-1, gc.ACS_URCORNER)
	w.MoveAddChar(y+height-1, x, gc.ACS_LLCORNER)
	w.MoveAddChar(y+height-1, x+width-1, gc.ACS_LRCORNER)

	// Perimeter length for the traveling highlight
	perim := 2*(width-2) + 2*(height-2)
	sparkPos := frame % (perim + 1)
	sparkLen := 8

	idx := 0

	// Top edge
	for i := 1; i < width-1; i++ {
		applySparkColor(w, idx, sparkPos, sparkLen, baseColor)
		w.MoveAddChar(y, x+i, gc.ACS_HLINE)
		idx++
	}
	// Right edge
	for i := 1; i < height-1; i++ {
		applySparkColor(w, idx, sparkPos, sparkLen, baseColor)
		w.MoveAddChar(y+i, x+width-1, gc.ACS_VLINE)
		idx++
	}
	// Bottom edge (reverse)
	for i := width - 2; i >= 1; i-- {
		applySparkColor(w, idx, sparkPos, sparkLen, baseColor)
		w.MoveAddChar(y+height-1, x+i, gc.ACS_HLINE)
		idx++
	}
	// Left edge (reverse)
	for i := height - 2; i >= 1; i-- {
		applySparkColor(w, idx, sparkPos, sparkLen, baseColor)
		w.MoveAddChar(y+i, x, gc.ACS_VLINE)
		idx++
	}

	w.ColorOn(cDefault)
}

func applySparkColor(w *gc.Window, idx, sparkPos, sparkLen int, baseColor int16) {
	dist := idx - sparkPos
	if dist < 0 {
		dist = -dist
	}
	if dist <= sparkLen {
		brightness := sparkLen - dist
		switch {
		case brightness >= 6:
			w.ColorOn(cDefault)
			w.AttrOn(gc.A_BOLD)
		case brightness >= 4:
			w.ColorOn(cCyan)
			w.AttrOn(gc.A_BOLD)
		case brightness >= 2:
			w.ColorOn(cDimCyan)
			w.AttrOff(gc.A_BOLD)
		default:
			w.ColorOn(baseColor)
			w.AttrOff(gc.A_BOLD)
		}
	} else {
		w.ColorOn(baseColor)
		w.AttrOff(gc.A_BOLD)
	}
}

func drawProgressBar(w *gc.Window, x, y, barW int, progress float64, frame int) {
	progress = clampFloat(progress, 0, 1)
	filled := int(math.Round(float64(barW) * progress))
	if progress > 0 && filled == 0 {
		filled = 1
	}
	if filled > barW {
		filled = barW
	}

	for i := 0; i < barW; i++ {
		switch {
		case i < filled-1:
			if (i+frame)%7 == 0 {
				w.ColorOn(cBarHead)
			} else {
				w.ColorOn(cBarFilled)
			}
			w.MovePrint(y, x+i, "█")
		case i == filled-1 && filled > 0:
			w.ColorOn(cBarHead)
			w.MovePrint(y, x+i, "▓")
		default:
			pulse := math.Sin(float64(i+frame) / 6.0)
			if pulse > 0.78 {
				w.ColorOn(cDimCyan)
				w.MovePrint(y, x+i, "░")
			} else {
				w.ColorOn(cDim)
				w.MovePrint(y, x+i, "░")
			}
		}
	}
	w.ColorOn(cDefault)
}

func drawStageList(w *gc.Window, stages []string, current, y, cx, maxW, frame int, spinChars []string, maxLines int) int {
	if len(stages) == 0 || maxLines <= 0 {
		return 0
	}

	if maxLines > len(stages) {
		maxLines = len(stages)
	}
	start := current - maxLines/2
	if start < 0 {
		start = 0
	}
	if start+maxLines > len(stages) {
		start = len(stages) - maxLines
	}
	if start < 0 {
		start = 0
	}

	for i := 0; i < maxLines; i++ {
		idx := start + i
		var prefix string
		switch {
		case idx < current:
			w.ColorOn(cGreen)
			prefix = fmt.Sprintf(" ✓  %02d ", idx+1)
		case idx == current:
			w.ColorOn(cYellow)
			w.AttrOn(gc.A_BOLD)
			spin := spinChars[frame%len(spinChars)]
			prefix = fmt.Sprintf(" %s  %02d ", spin, idx+1)
		default:
			w.ColorOn(cDim)
			prefix = fmt.Sprintf(" ·  %02d ", idx+1)
		}
		line := prefix + stages[idx]
		line = truncateText(line, maxW)
		printCentered(w, y+i, cx, line)
		if idx == current {
			w.AttrOff(gc.A_BOLD)
		}
	}
	return maxLines
}

func drawLog(w *gc.Window, buf []LogLine, startX, maxW, startY, height int) {
	if height <= 0 || len(buf) == 0 {
		return
	}
	start := len(buf) - height
	if start < 0 {
		start = 0
	}
	for i, line := range buf[start:] {
		if i >= height {
			break
		}
		text := line.text
		if len(text) > maxW {
			text = text[:maxW]
		}
		w.ColorOn(line.color)
		w.MovePrint(startY+i, startX, text)
	}
}

func appendLogAI(buf []LogLine, aiGen *aiLogGenerator, cfg Config, profile UpdateProfile, metrics UpdateMetrics, frame, stageIdx int, stage string) []LogLine {
	ts := time.Now().Format("15:04:05.000")
	if aiLine, ok := aiGen.getLine(); ok {
		aiLine.text = fmt.Sprintf("[%s] %s", ts, aiLine.text)
		buf = append(buf, aiLine)
	} else {
		line := buildOSLogLine(ts, cfg, profile, metrics, frame, stageIdx, stage)
		buf = append(buf, line)
	}
	if len(buf) > 128 {
		buf = buf[len(buf)-128:]
	}
	return buf
}

func appendLog(buf []LogLine, cfg Config, profile UpdateProfile, metrics UpdateMetrics, frame, stageIdx int, stage string) []LogLine {
	ts := time.Now().Format("15:04:05.000")
	line := buildOSLogLine(ts, cfg, profile, metrics, frame, stageIdx, stage)
	buf = append(buf, line)
	if len(buf) > 128 {
		buf = buf[len(buf)-128:]
	}
	return buf
}

func buildOSLogLine(ts string, cfg Config, profile UpdateProfile, metrics UpdateMetrics, frame, stageIdx int, stage string) LogLine {
	switch osFamily(cfg.OSName) {
	case "macos":
		return buildMacLogLine(ts, cfg, profile, metrics, frame, stageIdx, stage)
	case "windows":
		return buildWindowsLogLine(ts, cfg, profile, metrics, frame, stageIdx, stage)
	case "chromeos":
		return buildChromeOSLogLine(ts, cfg, profile, metrics, frame, stageIdx, stage)
	default:
		return buildLinuxLogLine(ts, cfg, profile, metrics, frame, stageIdx, stage)
	}
}

func buildLinuxLogLine(ts string, cfg Config, profile UpdateProfile, metrics UpdateMetrics, frame, stageIdx int, stage string) LogLine {
	flavor := linuxFlavor(cfg)
	system := systemLabel(cfg)
	switch frame % 10 {
	case 0:
		return LogLine{fmt.Sprintf("[%s] %s: synced %s metadata for %s", ts, flavor.Manager, flavor.Repo, cfg.Arch), cDim}
	case 1:
		return LogLine{fmt.Sprintf("[%s] %s: transaction prepared for %d packages on %s", ts, flavor.Manager, profile.PackageTotal, shortCPUModel(cfg.CPUModel, 20)), cDarkGreen}
	case 2:
		return LogLine{fmt.Sprintf("[%s] %s: verified %d/%d signatures in %s", ts, flavor.Manager, metrics.PackageCurrent, profile.PackageTotal, flavor.DBPath), cDarkGreen}
	case 3:
		return LogLine{fmt.Sprintf("[%s] %s: rebuilding initramfs for build %s (%d GB RAM)", ts, flavor.Initramfs, cfg.BuildID, maxInt(cfg.MemoryGB, 1)), cMagenta}
	case 4:
		return LogLine{fmt.Sprintf("[%s] %s: writing boot entries for %s on %s", ts, flavor.Bootloader, cfg.BuildID, volumeLabel(cfg)), cCyan}
	case 5:
		return LogLine{fmt.Sprintf("[%s] udev: coldplug sweep complete on %d logical CPUs", ts, maxInt(cfg.CPULogical, 1)), cDim}
	case 6:
		return LogLine{fmt.Sprintf("[%s] systemd: queued %s after %s", ts, flavor.Service, stage), cMagenta}
	case 7:
		return LogLine{fmt.Sprintf("[%s] fwupd: no pending capsule for %s; continuing userspace payload", ts, system), cDim}
	case 8:
		return LogLine{fmt.Sprintf("[%s] ldconfig: refreshed linker cache for %s libraries", ts, cfg.Arch), cDarkGreen}
	default:
		return LogLine{fmt.Sprintf("[%s] %s: phase %02d active, %d workers processing %s", ts, flavor.Manager, stageIdx+1, metrics.WorkerCount, stage), cCyan}
	}
}

func buildWindowsLogLine(ts string, cfg Config, profile UpdateProfile, metrics UpdateMetrics, frame, stageIdx int, stage string) LogLine {
	systemDrive := volumeLabel(cfg)
	if systemDrive == "/" {
		systemDrive = "C:"
	}
	switch frame % 10 {
	case 0:
		return LogLine{fmt.Sprintf("[%s] MoUsoCoreWorker: service scan accepted for build %s", ts, cfg.BuildID), cDim}
	case 1:
		return LogLine{fmt.Sprintf("[%s] CBS: staging %d packages for session %s", ts, profile.PackageTotal, metrics.SessionID), cDarkGreen}
	case 2:
		return LogLine{fmt.Sprintf("[%s] DISM: component store transaction prepared for %s", ts, cfg.Arch), cMagenta}
	case 3:
		return LogLine{fmt.Sprintf("[%s] TrustedInstaller: reserving %d GB rollback space on %s", ts, clampInt(maxInt(cfg.RootAvailGB/4, 2), 2, 24), systemDrive), cDim}
	case 4:
		return LogLine{fmt.Sprintf("[%s] SetupPlatform: compatibility scan passed on %s", ts, shortCPUModel(cfg.CPUModel, 24)), cCyan}
	case 5:
		return LogLine{fmt.Sprintf("[%s] WinSxS: manifest hashes validated %d/%d", ts, metrics.PackageCurrent, profile.PackageTotal), cDarkGreen}
	case 6:
		return LogLine{fmt.Sprintf("[%s] PnPUtil: queued inbox driver migration on %d logical processors", ts, maxInt(cfg.CPULogical, 1)), cMagenta}
	case 7:
		return LogLine{fmt.Sprintf("[%s] WaaSMedicSvc: servicing handoff complete after %s", ts, stage), cDim}
	case 8:
		return LogLine{fmt.Sprintf("[%s] Defender platform: signatures retained during cumulative servicing", ts), cDarkGreen}
	default:
		return LogLine{fmt.Sprintf("[%s] BCD: pending reboot actions scheduled for phase %02d", ts, stageIdx+1), cCyan}
	}
}

func buildMacLogLine(ts string, cfg Config, profile UpdateProfile, metrics UpdateMetrics, frame, stageIdx int, stage string) LogLine {
	system := systemLabel(cfg)
	switch frame % 10 {
	case 0:
		return LogLine{fmt.Sprintf("[%s] softwareupdated: accepted product catalog for %s %s", ts, cfg.OSName, cfg.OSVersion), cDim}
	case 1:
		return LogLine{fmt.Sprintf("[%s] MobileSoftwareUpdate: staging %.1f GB payload for %s", ts, profile.TotalBundleGB, system), cDarkGreen}
	case 2:
		return LogLine{fmt.Sprintf("[%s] apfs: snapshot com.apple.os.update-%04X created on system volume", ts, rand.Intn(0xFFFF)), cMagenta}
	case 3:
		return LogLine{fmt.Sprintf("[%s] syspolicyd: trust cache validation complete for build %s", ts, cfg.BuildID), cDarkGreen}
	case 4:
		return LogLine{fmt.Sprintf("[%s] kernelmanagerd: rebuilding boot collections for %s", ts, cfg.Arch), cCyan}
	case 5:
		return LogLine{fmt.Sprintf("[%s] launchd: queued daemon migration after %s", ts, stage), cMagenta}
	case 6:
		return LogLine{fmt.Sprintf("[%s] diskmanagementd: reserved %d GB free space on the system volume", ts, clampInt(maxInt(cfg.RootAvailGB/3, 4), 4, 48)), cDim}
	case 7:
		return LogLine{fmt.Sprintf("[%s] mds: Spotlight reindex token scheduled for post-install", ts), cDim}
	case 8:
		return LogLine{fmt.Sprintf("[%s] MobileSoftwareUpdate: validated %d/%d payload objects", ts, metrics.PackageCurrent, profile.PackageTotal), cDarkGreen}
	default:
		return LogLine{fmt.Sprintf("[%s] powerd: installation remains on %s during phase %02d", ts, metrics.PowerState, stageIdx+1), cCyan}
	}
}

func buildChromeOSLogLine(ts string, cfg Config, profile UpdateProfile, metrics UpdateMetrics, frame, stageIdx int, stage string) LogLine {
	switch frame % 10 {
	case 0:
		return LogLine{fmt.Sprintf("[%s] update_engine: Omaha response accepted for %s", ts, cfg.Arch), cDim}
	case 1:
		return LogLine{fmt.Sprintf("[%s] update_engine: delta payload %.1f GB staged to inactive slot", ts, profile.TotalBundleGB), cDarkGreen}
	case 2:
		return LogLine{fmt.Sprintf("[%s] verity: root hash tree verified for build %s", ts, cfg.BuildID), cDarkGreen}
	case 3:
		return LogLine{fmt.Sprintf("[%s] boot_control: slot B marked unbootable during staging", ts), cMagenta}
	case 4:
		return LogLine{fmt.Sprintf("[%s] dlcservice: synchronized %d payload objects", ts, metrics.PackageCurrent), cCyan}
	case 5:
		return LogLine{fmt.Sprintf("[%s] imageloader: ARC++ image refresh complete on %s", ts, systemLabel(cfg)), cDim}
	case 6:
		return LogLine{fmt.Sprintf("[%s] cros-disks: stateful partition trim queued on %s", ts, volumeLabel(cfg)), cMagenta}
	case 7:
		return LogLine{fmt.Sprintf("[%s] tpm_managerd: update metadata sealed for pending reboot", ts), cDarkGreen}
	case 8:
		return LogLine{fmt.Sprintf("[%s] update_engine: %s (%d workers active)", ts, stage, metrics.WorkerCount), cCyan}
	default:
		return LogLine{fmt.Sprintf("[%s] boot_control: slot B marked successful pending restart", ts), cDim}
	}
}

type distroFlavor struct {
	Kind       string
	Manager    string
	Repo       string
	DBPath     string
	Initramfs  string
	Bootloader string
	Service    string
}

func linuxFlavor(cfg Config) distroFlavor {
	key := strings.ToLower(strings.Join([]string{cfg.DistroID, cfg.DistroLike, cfg.OSName}, " "))
	switch {
	case containsAny(key, "ubuntu", "debian", "mint", "linuxmint", "pop", "elementary", "zorin", "neon", "devuan"):
		return distroFlavor{"apt", "apt/dpkg", "deb archive", "/var/lib/dpkg", "update-initramfs", "update-grub", "apt-daily-upgrade.service"}
	case containsAny(key, "fedora", "rhel", "rocky", "alma", "centos", "ol", "nobara"):
		return distroFlavor{"dnf", "dnf/rpm", "rpm-md", "/var/lib/rpm", "dracut", "grub2-mkconfig", "dnf-makecache.service"}
	case containsAny(key, "arch", "manjaro", "endeavouros", "arco"):
		return distroFlavor{"pacman", "pacman", "sync db", "/var/lib/pacman", "mkinitcpio", "grub-mkconfig", "systemd-udevd.service"}
	case containsAny(key, "opensuse", "suse", "sles"):
		return distroFlavor{"zypper", "zypper", "repomd", "/var/lib/zypp", "dracut", "grub2-mkconfig", "wicked.service"}
	case containsAny(key, "void"):
		return distroFlavor{"xbps", "xbps", "xbps repodata", "/var/db/xbps", "dracut", "grub-mkconfig", "socklog-unix.service"}
	case containsAny(key, "alpine"):
		return distroFlavor{"apk", "apk", "APKINDEX", "/lib/apk/db", "mkinitfs", "update-extlinux", "mdev.service"}
	case containsAny(key, "gentoo", "funtoo"):
		return distroFlavor{"emerge", "emerge", "ebuild tree", "/var/db/pkg", "genkernel", "grub-mkconfig", "openrc"}
	case containsAny(key, "nixos", "nixos-unstable"):
		return distroFlavor{"nix", "nix", "nix channels", "/nix/var/nix/db", "nixos-rebuild boot", "bootctl", "nix-daemon.service"}
	case containsAny(key, "slackware"):
		return distroFlavor{"slackpkg", "slackpkg", "package mirror", "/var/lib/pkgtools", "mkinitrd", "lilo", "rc.M"}
	case containsAny(key, "solus"):
		return distroFlavor{"eopkg", "eopkg", "eopkg index", "/var/lib/eopkg", "clr-boot-manager", "clr-boot-manager update", "systemd-udevd.service"}
	case containsAny(key, "guix"):
		return distroFlavor{"guix", "guix", "channel metadata", "/var/guix/db", "guix system", "grub-mkconfig", "guix-daemon.service"}
	case containsAny(key, "clear-linux", "clear linux"):
		return distroFlavor{"swupd", "swupd", "manifest metadata", "/var/lib/swupd", "clr-boot-manager", "clr-boot-manager update", "swupd-update.service"}
	default:
		return distroFlavor{"generic", "packagekit", "repository metadata", "/var/lib/PackageKit", "dracut", "bootctl", "systemd-udevd.service"}
	}
}

func getLinuxStages(cfg Config) []string {
	flavor := linuxFlavor(cfg)
	switch flavor.Kind {
	case "apt":
		return []string{
			"Updating APT package lists",
			"Validating InRelease signatures",
			"Resolving dpkg dependency graph",
			"Downloading .deb archives",
			"Unpacking .deb packages",
			"Configuring upgraded packages",
			"Running ldconfig and libc triggers",
			"Regenerating initramfs",
			"Updating GRUB menu entries",
			"Cleaning APT cache",
		}
	case "dnf":
		return []string{
			"Refreshing DNF repository metadata",
			"Downloading RPM metalinks",
			"Resolving RPM transaction set",
			"Downloading RPM packages",
			"Importing GPG signatures",
			"Installing updated packages",
			"Rebuilding RPM database",
			"Running dracut",
			"Updating GRUB2 configuration",
			"Cleaning DNF cache",
		}
	case "pacman":
		return []string{
			"Synchronizing pacman databases",
			"Ranking package mirrors",
			"Resolving package conflicts",
			"Downloading package archives",
			"Verifying package signatures",
			"Installing pacman transactions",
			"Running mkinitcpio presets",
			"Updating GRUB entries",
			"Refreshing shared library cache",
			"Cleaning pacman cache",
		}
	case "zypper":
		return []string{
			"Refreshing zypper repositories",
			"Resolving solver rules",
			"Downloading RPM payloads",
			"Validating package signatures",
			"Installing selected patches",
			"Refreshing zypp cache",
			"Regenerating initramfs with dracut",
			"Updating GRUB2 entries",
			"Applying post-trans scripts",
			"Cleaning zypper cache",
		}
	case "xbps":
		return []string{
			"Synchronizing xbps repositories",
			"Resolving xbps transaction set",
			"Downloading xbps packages",
			"Verifying package signatures",
			"Unpacking xbps archives",
			"Configuring installed packages",
			"Running dracut",
			"Updating GRUB entries",
			"Refreshing dynamic linker cache",
			"Cleaning xbps cache",
		}
	case "apk":
		return []string{
			"Fetching APKINDEX metadata",
			"Resolving world dependencies",
			"Downloading .apk archives",
			"Verifying package signatures",
			"Installing Alpine packages",
			"Running post-upgrade hooks",
			"Rebuilding mkinitfs images",
			"Updating extlinux configuration",
			"Refreshing shared caches",
			"Cleaning APK cache",
		}
	case "emerge":
		return []string{
			"Synchronizing Portage tree",
			"Calculating USE flag rebuilds",
			"Fetching distfiles",
			"Verifying Manifest entries",
			"Merging ebuild packages",
			"Running preserved-rebuild",
			"Updating configuration files",
			"Generating initramfs",
			"Updating GRUB entries",
			"Cleaning obsolete distfiles",
		}
	case "nix":
		return []string{
			"Refreshing Nix channels",
			"Evaluating derivations",
			"Downloading NAR substitutes",
			"Verifying store paths",
			"Building system closures",
			"Activating new system profile",
			"Registering boot generation",
			"Updating bootloader profile",
			"Collecting stale paths",
			"Finalizing nixos-rebuild",
		}
	case "slackpkg":
		return []string{
			"Refreshing slackpkg mirror list",
			"Downloading package manifests",
			"Verifying GPG signatures",
			"Installing upgraded .txz packages",
			"Running package setup scripts",
			"Rebuilding initrd",
			"Updating lilo configuration",
			"Refreshing shared library cache",
			"Applying post-install tasks",
			"Cleaning slackpkg cache",
		}
	case "eopkg":
		return []string{
			"Refreshing eopkg index",
			"Resolving package actions",
			"Downloading eopkg archives",
			"Verifying package checksums",
			"Applying Solus package transactions",
			"Rebuilding initramfs",
			"Updating clr-boot-manager",
			"Refreshing linker cache",
			"Running system triggers",
			"Cleaning eopkg cache",
		}
	case "guix":
		return []string{
			"Pulling Guix channels",
			"Resolving system derivations",
			"Downloading substitutes",
			"Verifying nar signatures",
			"Building new system generation",
			"Activating Guix profile",
			"Reconfiguring services",
			"Updating boot entries",
			"Pruning obsolete generations",
			"Finalizing guix system reconfigure",
		}
	case "swupd":
		return []string{
			"Refreshing swupd manifests",
			"Verifying bundle metadata",
			"Downloading binary deltas",
			"Applying bundle updates",
			"Updating filesystem version files",
			"Rebuilding initramfs assets",
			"Updating clr-boot-manager",
			"Refreshing boot entries",
			"Running post-update tasks",
			"Cleaning swupd cache",
		}
	default:
		return []string{
			"Refreshing package indexes",
			"Negotiating mirror priority",
			"Resolving dependency graph",
			"Downloading signed packages",
			"Verifying repository signatures",
			"Expanding package archives",
			"Installing userspace libraries",
			"Updating kernel image",
			"Installing kernel headers",
			"Running maintainer scripts",
			"Refreshing udev rules",
			"Rebuilding initramfs",
			"Updating bootloader entries",
			"Recomputing shared caches",
			"Applying post-install triggers",
			"Cleaning package cache",
		}
	}
}

func osFamily(osName string) string {
	lower := strings.ToLower(osName)
	switch {
	case lower == "macos", lower == "mac os x":
		return "macos"
	case lower == "windows" || strings.Contains(lower, "windows"):
		return "windows"
	case lower == "chromeos" || lower == "chrome os" || strings.Contains(lower, "chrome os"):
		return "chromeos"
	default:
		return "linux"
	}
}

func systemLabel(cfg Config) string {
	if cfg.DeviceName != "" {
		return cfg.DeviceName
	}
	if cfg.Hostname != "" {
		return cfg.Hostname
	}
	return "local-system"
}

func volumeLabel(cfg Config) string {
	if cfg.RootDevice != "" {
		return cfg.RootDevice
	}
	if runtime.GOOS == "windows" {
		return "C:"
	}
	return "/"
}

// ====================== STAGES ======================

func getStages(cfg Config) []string {
	switch osFamily(cfg.OSName) {
	case "macos":
		return []string{
			"Contacting Apple Software Update",
			"Downloading product metadata",
			"Validating update signature",
			"Preparing APFS recovery snapshot",
			"Staging system volume payload",
			"Installing platform frameworks",
			"Refreshing dyld shared cache",
			"Rebuilding kernel collections",
			"Applying Rapid Security Response assets",
			"Migrating launch services",
			"Updating firmware support files",
			"Reindexing Spotlight metadata",
			"Optimizing system storage",
			"Committing startup policy",
			"Scheduling automatic restart",
			"Finalizing macOS update",
		}
	case "chromeos":
		return []string{
			"Contacting update server",
			"Downloading signed delta image",
			"Verifying payload checksum",
			"Mapping inactive rootfs",
			"Writing system image to partition B",
			"Updating verified boot metadata",
			"Syncing firmware assets",
			"Refreshing browser image",
			"Applying lacros update",
			"Updating ARC++ runtime",
			"Refreshing crostini containers",
			"Repairing policy cache",
			"Rebuilding shader cache",
			"Optimizing stateful partition",
			"Running post-install hooks",
			"Marking slot as bootable",
			"Preparing restart",
		}
	case "windows":
		return []string{
			"Checking Windows Update service health",
			"Downloading cumulative update metadata",
			"Validating servicing stack",
			"Staging Windows Update packages",
			"Creating system restore checkpoint",
			"Installing cumulative update payload",
			"Applying .NET servicing updates",
			"Updating inbox drivers",
			"Refreshing Microsoft Defender platform",
			"Configuring Windows scheduled tasks",
			"Optimizing WinSxS manifests",
			"Committing registry transaction logs",
			"Sealing boot-critical files",
			"Scheduling pending reboot actions",
			"Cleaning superseded update payloads",
			"Finalizing Windows Update",
		}
	default:
		return getLinuxStages(cfg)
	}
}

func getUpdateProfile(cfg Config) UpdateProfile {
	lowerOS := osFamily(cfg.OSName)
	specScale := clampFloat(float64(maxInt(cfg.MemoryGB, 4))/16.0, 0.25, 3.0)
	diskScale := clampFloat(float64(maxInt(cfg.RootDiskGB, 64))/512.0, 0.05, 2.5)

	switch lowerOS {
	case "macos":
		return withSpecScale(UpdateProfile{
			Channel:        "stable",
			TotalBundleGB:  7.6,
			PackageTotal:   820,
			Headline:       "Apple Software Update is preparing a sealed system volume",
			Advisory:       "Keep the device on external power. APFS snapshots, firmware support files, and an automatic restart are expected.",
			PayloadLabel:   "Payload",
			IntegrityLabel: "seal verified / signed",
		}, specScale, diskScale)
	case "chromeos":
		return withSpecScale(UpdateProfile{
			Channel:        "stable",
			TotalBundleGB:  4.2,
			PackageTotal:   690,
			Headline:       "Updating the inactive slot with a verified system image",
			Advisory:       "Network and storage usage may spike while the standby partition is synchronized.",
			PayloadLabel:   "Image",
			IntegrityLabel: "verified boot / hash tree",
		}, specScale, diskScale)
	case "windows":
		return withSpecScale(UpdateProfile{
			Channel:        "production",
			TotalBundleGB:  5.4,
			PackageTotal:   1120,
			Headline:       "Windows Update is servicing the component store and staging cumulative payloads",
			Advisory:       "Leave the machine plugged in while rollback data and pending reboot actions are written.",
			PayloadLabel:   "Bundle",
			IntegrityLabel: "catalog signed / CBS",
		}, specScale, diskScale)
	default:
		return withSpecScale(UpdateProfile{
			Channel:        "stable",
			TotalBundleGB:  2.2,
			PackageTotal:   420,
			Headline:       "Applying signed packages and rebuilding boot assets",
			Advisory:       "Power, network, and disk activity can increase while package triggers and initramfs are rebuilt.",
			PayloadLabel:   "Archive",
			IntegrityLabel: "gpg signed / sha256",
		}, specScale, diskScale)
	}
}

func withSpecScale(profile UpdateProfile, specScale, diskScale float64) UpdateProfile {
	profile.TotalBundleGB += specScale*0.35 + diskScale*0.25
	profile.PackageTotal += int(math.Round(specScale*42 + diskScale*18))
	return profile
}

func buildUpdateMetrics(cfg Config, profile UpdateProfile, progress float64, frame, stageIdx, totalStages int, sessionID string) UpdateMetrics {
	progress = clampFloat(progress, 0, 1)
	stageProgress := progress * float64(totalStages)
	stageProgress -= math.Floor(stageProgress)
	if progress >= 1 {
		stageProgress = 1
	}

	bundleJitter := 0.025 * math.Sin(float64(frame)/9.0)
	bundleCurrent := clampFloat(profile.TotalBundleGB*(progress+bundleJitter), 0, profile.TotalBundleGB)

	packageJitter := 0.018 * math.Sin(float64(frame)/13.0)
	packageCurrent := int(math.Round(float64(profile.PackageTotal) * clampFloat(progress+packageJitter, 0, 1)))
	if progress > 0 && packageCurrent == 0 {
		packageCurrent = 1
	}
	if packageCurrent > profile.PackageTotal {
		packageCurrent = profile.PackageTotal
	}

	threadCap := clampInt(maxInt(cfg.CPULogical, 2), 2, 16)
	workerCount := clampInt(threadCap-1+stageIdx%3, 2, threadCap)
	netBase := 72 + clampInt(maxInt(cfg.MemoryGB, 4)*3, 12, 96)
	netMbps := clampInt(netBase+int((1-progress)*74)+int((1+math.Sin(float64(frame)/7.0))*12), 24, 1000)
	diskBase := diskThroughputBase(cfg)
	diskMBps := clampInt(diskBase+int(progress*float64(maxInt(diskBase/2, 24)))+int((1+math.Sin(float64(frame)/10.0))*14), 24, 2400)
	powerState := cfg.PowerState
	if powerState == "" {
		powerState = "AC"
	}

	return UpdateMetrics{
		SessionID:       sessionID,
		BundleCurrentGB: bundleCurrent,
		PackageCurrent:  packageCurrent,
		NetMbps:         netMbps,
		DiskMBps:        diskMBps,
		WorkerCount:     workerCount,
		StageProgress:   clampFloat(stageProgress, 0, 1),
		Integrity:       profile.IntegrityLabel,
		PowerState:      powerState,
	}
}

func diskThroughputBase(cfg Config) int {
	device := strings.ToLower(cfg.RootDevice)
	switch {
	case strings.Contains(device, "nvme"):
		return 540
	case strings.Contains(device, "mmc"):
		return 82
	case strings.Contains(device, "sd"), strings.Contains(device, "vd"), strings.Contains(device, "xvd"):
		return 126
	default:
		return 164
	}
}

func formatThreadBadge(cfg Config) string {
	switch {
	case cfg.CPUCores > 0 && cfg.CPULogical > 0:
		return fmt.Sprintf("%dC / %dT", cfg.CPUCores, cfg.CPULogical)
	case cfg.CPULogical > 0:
		return fmt.Sprintf("%d THREADS", cfg.CPULogical)
	case cfg.Arch != "":
		return strings.ToUpper(cfg.Arch)
	default:
		return ""
	}
}

func formatSpecSummary(cfg Config) string {
	parts := make([]string, 0, 4)
	if cpu := shortCPUModel(cfg.CPUModel, 22); cpu != "" {
		parts = append(parts, cpu)
	}
	if cfg.MemoryGB > 0 {
		parts = append(parts, fmt.Sprintf("%d GB RAM", cfg.MemoryGB))
	}
	if cfg.Arch != "" {
		parts = append(parts, cfg.Arch)
	}
	if cfg.RootDiskGB > 0 {
		disk := fmt.Sprintf("Root %d GB", cfg.RootDiskGB)
		if cfg.RootDevice != "" {
			disk = fmt.Sprintf("%s %d GB", shortDeviceName(cfg.RootDevice), cfg.RootDiskGB)
		}
		parts = append(parts, disk)
	}
	if len(parts) == 0 {
		return "Local system profile"
	}
	return strings.Join(parts, " | ")
}

func shortCPUModel(model string, max int) string {
	model = cleanCPUModel(model)
	if model == "" {
		return ""
	}
	return truncateText(model, max)
}

func shortDeviceName(device string) string {
	if device == "" {
		return ""
	}
	return strings.TrimPrefix(device, "/dev/")
}

func drawCenteredBadges(w *gc.Window, y, cx, maxW int, badges []Badge) {
	total := -1
	for _, badge := range badges {
		total += len(badge.text) + 3
	}
	if total > maxW {
		for len(badges) > 1 && total > maxW {
			total -= len(badges[len(badges)-1].text) + 3
			badges = badges[:len(badges)-1]
		}
	}
	if len(badges) == 0 {
		return
	}
	x := cx - total/2
	if x < 1 {
		x = 1
	}
	drawBadgesAt(w, y, x, x+total, cDefault, badges)
}

func drawBadgesAt(w *gc.Window, y, x, maxX int, spacerColor int16, badges []Badge) int {
	cur := x
	for i, badge := range badges {
		segW := len(badge.text) + 2
		if i > 0 {
			segW++
		}
		if cur+segW > maxX {
			break
		}
		if i > 0 {
			w.ColorOn(spacerColor)
			w.MovePrint(y, cur, " ")
			cur++
		}
		w.ColorOn(badge.color)
		w.AttrOn(gc.A_BOLD)
		w.MovePrint(y, cur, " "+badge.text+" ")
		w.AttrOff(gc.A_BOLD)
		cur += len(badge.text) + 2
	}
	w.ColorOn(cDefault)
	return cur
}

func drawTopStatusBar(w *gc.Window, cols int, now time.Time, cfg Config, profile UpdateProfile, metrics UpdateMetrics) {
	y := 1
	w.ColorOn(cDim)
	for x := 1; x < cols-1; x++ {
		w.MovePrint(y, x, " ")
	}

	badges := []Badge{
		{text: "LIVE", color: cBarFilled},
		{text: "TRACK " + strings.ToUpper(profile.Channel), color: cInfoBadge},
		{text: "POWER " + metrics.PowerState, color: cStageBadge},
	}
	leftX := drawBadgesAt(w, y, 2, cols-2, cDim, badges)

	details := fmt.Sprintf(" %s %s  build %s", cfg.OSName, cfg.OSVersion, cfg.BuildID)
	if cfg.DeviceName != "" {
		details += "  " + cfg.DeviceName
	}
	if cfg.Arch != "" {
		details += "  " + cfg.Arch
	}
	if cpu := shortCPUModel(cfg.CPUModel, 18); cpu != "" {
		details += "  " + cpu
	}
	right := now.Format("15:04:05")
	rightX := cols - 1 - len(right)
	if rightX < 1 {
		rightX = 1
	}
	if leftX+1 < rightX {
		w.ColorOn(cDimCyan)
		w.MovePrint(y, leftX+1, truncateText(details, rightX-leftX-2))
	}
	w.ColorOn(cCyan)
	w.AttrOn(gc.A_BOLD)
	w.MovePrint(y, rightX, right)
	w.AttrOff(gc.A_BOLD)
}

func drawBottomStatusBar(w *gc.Window, y, cols int, elapsed float64, remaining time.Duration, progress float64, stageIdx, totalStages, packageTotal int, metrics UpdateMetrics) {
	w.ColorOn(cDim)
	for x := 1; x < cols-1; x++ {
		w.MovePrint(y, x, " ")
	}

	badges := []Badge{
		{text: "APPLYING", color: cBarFilled},
		{text: fmt.Sprintf("PHASE %02d/%02d", stageIdx+1, totalStages), color: cStageBadge},
		{text: fmt.Sprintf("PKG %d/%d", metrics.PackageCurrent, packageTotal), color: cBarHead},
	}
	leftX := drawBadgesAt(w, y, 2, cols-2, cDim, badges)

	leftText := fmt.Sprintf(" Overall %.0f%%  Stage %.0f%%  Session %s", progress*100, metrics.StageProgress*100, metrics.SessionID)
	rightText := fmt.Sprintf("NET %dMb/s | DISK %dMB/s | ETA %s | ELAPSED %s",
		metrics.NetMbps, metrics.DiskMBps, formatRemaining(remaining), formatClock(int(elapsed)))

	rightX := cols - 1 - len(rightText)
	if rightX < leftX+2 {
		rightText = truncateText(rightText, cols-leftX-4)
		rightX = cols - 1 - len(rightText)
	}
	if leftX+1 < rightX {
		w.ColorOn(cDimCyan)
		w.MovePrint(y, leftX+1, truncateText(leftText, rightX-leftX-2))
	}
	if rightX > 1 {
		w.ColorOn(cCyan)
		w.MovePrint(y, rightX, rightText)
	}
}

func formatRemaining(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func formatClock(totalSeconds int) string {
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func truncateText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(s, value) {
			return true
		}
	}
	return false
}

// ====================== AI LOG GENERATOR ======================

type aiLogGenerator struct {
	mu       sync.Mutex
	buf      []LogLine
	cfg      Config
	profile  UpdateProfile
	apiKey   string
	client   *http.Client
	active   bool
	fetching bool
}

func newAILogGenerator(cfg Config, profile UpdateProfile) *aiLogGenerator {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" || cfg.NoAI {
		return &aiLogGenerator{active: false}
	}
	gen := &aiLogGenerator{
		cfg:     cfg,
		profile: profile,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
		active:  true,
	}
	// Pre-fetch initial batch
	go gen.fetchBatch(0, 0, "Initializing")
	return gen
}

func (g *aiLogGenerator) getLine() (LogLine, bool) {
	if !g.active {
		return LogLine{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.buf) == 0 {
		return LogLine{}, false
	}
	line := g.buf[0]
	g.buf = g.buf[1:]
	return line, true
}

func (g *aiLogGenerator) ensureBuffer(stageIdx int, totalStages int, stage string) {
	if !g.active {
		return
	}
	g.mu.Lock()
	needFetch := len(g.buf) < 5 && !g.fetching
	g.mu.Unlock()
	if needFetch {
		go g.fetchBatch(stageIdx, totalStages, stage)
	}
}

type openAIRequest struct {
	Model       string             `json:"model"`
	Messages    []openAIMessage    `json:"messages"`
	Temperature float64            `json:"temperature"`
	MaxTokens   int                `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (g *aiLogGenerator) fetchBatch(stageIdx, totalStages int, stage string) {
	g.mu.Lock()
	if g.fetching {
		g.mu.Unlock()
		return
	}
	g.fetching = true
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		g.fetching = false
		g.mu.Unlock()
	}()

	family := osFamily(g.cfg.OSName)
	flavor := linuxFlavor(g.cfg)

	var deviceDetails strings.Builder
	deviceDetails.WriteString(fmt.Sprintf("OS: %s %s (family: %s)\n", g.cfg.OSName, g.cfg.OSVersion, family))
	deviceDetails.WriteString(fmt.Sprintf("Build ID: %s\n", g.cfg.BuildID))
	deviceDetails.WriteString(fmt.Sprintf("Hostname: %s\n", g.cfg.Hostname))
	if g.cfg.DeviceName != "" {
		deviceDetails.WriteString(fmt.Sprintf("Device Model: %s\n", g.cfg.DeviceName))
	}
	deviceDetails.WriteString(fmt.Sprintf("Architecture: %s\n", g.cfg.Arch))
	if g.cfg.CPUModel != "" {
		deviceDetails.WriteString(fmt.Sprintf("CPU: %s (%d cores, %d threads)\n", g.cfg.CPUModel, maxInt(g.cfg.CPUCores, 1), maxInt(g.cfg.CPULogical, 1)))
	}
	deviceDetails.WriteString(fmt.Sprintf("RAM: %d GB\n", maxInt(g.cfg.MemoryGB, 1)))
	if g.cfg.RootDevice != "" {
		deviceDetails.WriteString(fmt.Sprintf("Root Disk: %s (%d GB total, %d GB available)\n", g.cfg.RootDevice, g.cfg.RootDiskGB, g.cfg.RootAvailGB))
	}
	deviceDetails.WriteString(fmt.Sprintf("Power: %s\n", g.cfg.PowerState))
	if g.cfg.DistroID != "" {
		deviceDetails.WriteString(fmt.Sprintf("Distro ID: %s\n", g.cfg.DistroID))
	}
	if g.cfg.DistroLike != "" {
		deviceDetails.WriteString(fmt.Sprintf("Distro Like: %s\n", g.cfg.DistroLike))
	}
	if family == "linux" {
		deviceDetails.WriteString(fmt.Sprintf("Package Manager: %s (repo: %s, db: %s)\n", flavor.Manager, flavor.Repo, flavor.DBPath))
		deviceDetails.WriteString(fmt.Sprintf("Initramfs Tool: %s, Bootloader: %s\n", flavor.Initramfs, flavor.Bootloader))
	}

	var exampleLines string
	switch family {
	case "macos":
		exampleLines = `softwareupdated: accepted product catalog for macOS 14.5
MobileSoftwareUpdate: staging 7.6 GB payload for MacBookPro18,1
apfs: snapshot com.apple.os.update-3FA2 created on system volume
syspolicyd: trust cache validation complete for build 23F79
kernelmanagerd: rebuilding boot collections for arm64
launchd: queued daemon migration after Installing platform frameworks
diskmanagementd: reserved 12 GB free space on the system volume
mds: Spotlight reindex token scheduled for post-install`
	case "windows":
		exampleLines = `MoUsoCoreWorker: service scan accepted for build 22631
CBS: staging 1120 packages for session 142205-A3F1
DISM: component store transaction prepared for x86_64
TrustedInstaller: reserving 6 GB rollback space on C:
SetupPlatform: compatibility scan passed on Intel Core i7-12700K
WinSxS: manifest hashes validated 412/1120
PnPUtil: queued inbox driver migration on 16 logical processors
Defender platform: signatures retained during cumulative servicing`
	case "chromeos":
		exampleLines = `update_engine: Omaha response accepted for x86_64
update_engine: delta payload 4.2 GB staged to inactive slot
verity: root hash tree verified for build 15572.50.0
boot_control: slot B marked unbootable during staging
dlcservice: synchronized 312 payload objects
imageloader: ARC++ image refresh complete on ASUS Chromebook
tpm_managerd: update metadata sealed for pending reboot`
	default:
		exampleLines = fmt.Sprintf(`%s: synchronized core database for %s
%s: transaction prepared for 420 packages on %s
systemd: queued systemd-udevd.service after Resolving dependency graph
udev: coldplug sweep complete on 8 logical CPUs
%s: rebuilding initramfs for build 6.8.1-arch1
%s: writing boot entries for 6.8.1-arch1 on /dev/nvme0n1p1
ldconfig: refreshed linker cache for x86_64 libraries
fwupd: no pending capsule for local-system; continuing userspace payload`,
			flavor.Manager, flavor.Repo, flavor.Manager, g.cfg.Arch,
			flavor.Initramfs, flavor.Bootloader)
	}

	systemPrompt := fmt.Sprintf(`You generate realistic system update log lines. Here is the full device profile:

%s
Update context: %s channel, %d total packages, %.1f GB bundle, integrity: %s.

STRICT FORMAT — every line MUST be:
  processname: actual log message

Examples of correct lines for %s:
%s

Rules:
- Output exactly 25 log lines, one per line
- Every line MUST follow the format "daemonname: message text" — no brackets, no prefixes, no timestamps
- Timestamps like [HH:MM:SS.mmm] are prepended automatically by the caller — NEVER include them
- Reference real daemon names, real file paths, real package names, and real system services specific to this exact OS and distro
- Use the actual device details above (CPU model, disk device, hostname, build ID, arch, RAM amount) in log lines where relevant
- Vary between: package downloads/installs, kernel/driver operations, service restarts, cache rebuilds, integrity/signature checks, firmware updates, initramfs rebuilds, bootloader updates, library relinking
- Use technical jargon and log format appropriate for %s %s
- Do NOT number the lines or use bullet points
- Do NOT include empty lines
- Keep each line under 100 characters`,
		deviceDetails.String(),
		g.profile.Channel, g.profile.PackageTotal, g.profile.TotalBundleGB, g.profile.IntegrityLabel,
		family, exampleLines, g.cfg.OSName, g.cfg.OSVersion)

	userPrompt := fmt.Sprintf("Generate 25 log lines for update stage: \"%s\" (phase %d/%d)", stage, stageIdx+1, maxInt(totalStages, 1))

	reqBody := openAIRequest{
		Model: "gpt-4o-mini",
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.9,
		MaxTokens:   2000,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		g.mu.Lock()
		g.active = false
		g.mu.Unlock()
		return
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		g.mu.Lock()
		g.active = false
		g.mu.Unlock()
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		g.mu.Lock()
		g.active = false
		g.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		g.mu.Lock()
		g.active = false
		g.mu.Unlock()
		return
	}

	var aiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		g.mu.Lock()
		g.active = false
		g.mu.Unlock()
		return
	}

	if len(aiResp.Choices) == 0 {
		g.mu.Lock()
		g.active = false
		g.mu.Unlock()
		return
	}

	content := aiResp.Choices[0].Message.Content
	lines := strings.Split(content, "\n")

	colors := []int16{cDim, cDarkGreen, cMagenta, cCyan, cDim, cDarkGreen, cMagenta, cCyan, cDim, cDarkGreen}

	var newLines []LogLine
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip any leading numbering like "1. " or "- "
		if len(line) > 2 && line[0] >= '0' && line[0] <= '9' {
			if idx := strings.Index(line, " "); idx > 0 && idx < 4 {
				stripped := strings.TrimLeft(line[idx:], " .")
				if stripped != "" {
					line = stripped
				}
			}
		}
		line = strings.TrimPrefix(line, "- ")
		color := colors[i%len(colors)]
		newLines = append(newLines, LogLine{text: line, color: color})
	}

	if len(newLines) > 0 {
		g.mu.Lock()
		g.buf = append(g.buf, newLines...)
		g.mu.Unlock()
	}
}

// ====================== COMPLETION ======================

func showComplete(stdscr *gc.Window, cfg Config) {
	rows, cols := stdscr.MaxYX()
	cy := rows / 2
	cx := cols / 2

	// Scanline wipe
	for y := 0; y < rows; y++ {
		stdscr.ColorOn(cGreen)
		stdscr.MovePrint(y, 0, strings.Repeat("█", cols))
		stdscr.Refresh()
		time.Sleep(10 * time.Millisecond)
	}

	stdscr.Erase()

	check := []string{
		"                      ██   ",
		"                     ██    ",
		"                    ██     ",
		"                   ██      ",
		"                  ██       ",
		"      ██        ██         ",
		"       ██      ██          ",
		"        ██    ██           ",
		"         ██  ██            ",
		"          ████             ",
		"           ██              ",
	}

	// Fade-in: draw each line character by character
	stdscr.ColorOn(cGreen)
	stdscr.AttrOn(gc.A_BOLD)
	startY := cy - len(check)/2 - 2
	for i, ln := range check {
		lineX := cx - len(ln)/2
		if lineX < 0 {
			lineX = 0
		}
		for j, ch := range ln {
			if ch != ' ' {
				stdscr.MovePrint(startY+i, lineX+j, string(ch))
			}
		}
		stdscr.Refresh()
		time.Sleep(30 * time.Millisecond)
	}

	// Fade-in message character by character
	msg := fmt.Sprintf("%s %s Update Complete!", cfg.OSName, cfg.OSVersion)
	msgY := startY + len(check) + 2
	msgX := cx - len(msg)/2
	if msgX < 0 {
		msgX = 0
	}
	for j, ch := range msg {
		stdscr.MovePrint(msgY, msgX+j, string(ch))
		stdscr.Refresh()
		time.Sleep(20 * time.Millisecond)
	}
	stdscr.AttrOff(gc.A_BOLD)

	stdscr.ColorOn(cCyan)
	subMsg := "System will restart momentarily..."
	subY := msgY + 2
	subX := cx - len(subMsg)/2
	if subX < 0 {
		subX = 0
	}
	for j, ch := range subMsg {
		stdscr.MovePrint(subY, subX+j, string(ch))
		stdscr.Refresh()
		time.Sleep(15 * time.Millisecond)
	}

	for i := 3; i > 0; i-- {
		time.Sleep(800 * time.Millisecond)
		stdscr.ColorOn(cMagenta)
		printCentered(stdscr, subY+2, cx, fmt.Sprintf("%s %d %s",
			strings.Repeat("·", 4-i), i, strings.Repeat("·", 4-i)))
		stdscr.Refresh()
	}

	stdscr.Erase()
	stdscr.Refresh()
	time.Sleep(300 * time.Millisecond)
}
