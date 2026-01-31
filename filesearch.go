package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// ANSI Color Codes
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"

	// ClearLine wipes the entire current line
	ClearLine = "\033[2K"
)

// Config holds command line arguments
type Config struct {
	Dir          string
	Day          int
	Month        int
	Year         int
	AllDate      string
	Recursive    bool
	Extensions   string
	OutputFormat string
}

// FileResult struct handles the output data structure
type FileResult struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastModified string    `json:"last_modified"`
	ModTimeRaw   time.Time `json:"-"`
	Size         int64     `json:"size_bytes"`
}

var (
	scannedCount int
	matchCount   int
	showLive     bool
)

func main() {
	// 1. Parse Flags
	config := parseFlags()

	// Determine if we show live output
	showLive = (config.OutputFormat == "json" || config.OutputFormat == "md")

	// 2. Validate Directory
	if _, err := os.Stat(config.Dir); os.IsNotExist(err) {
		printError(fmt.Sprintf("Directory '%s' does not exist.", config.Dir))
		os.Exit(1)
	}

	// 3. Prepare Extension Map
	allowedExts := make(map[string]bool)
	if config.Extensions != "" {
		parts := strings.Split(config.Extensions, ",")
		for _, p := range parts {
			ext := strings.ToLower(strings.TrimSpace(p))
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			allowedExts[ext] = true
		}
	}

	printHeader()
	printInfo(fmt.Sprintf("Target: %s", config.Dir))

	if config.Recursive {
		printInfo("Mode: Deep Scan (Recursive Traversal)")
	} else {
		printInfo("Mode: Flat Scan (Current Folder Only)")
	}

	if config.Extensions != "" {
		printInfo(fmt.Sprintf("Filter: %s files", config.Extensions))
	}

	printDateFilter(config)
	fmt.Println() // Space before scanning starts

	// 4. Run Search
	var results []FileResult
	var err error

	startTime := time.Now()

	if config.Recursive {
		results, err = scanRecursive(config, allowedExts)
	} else {
		results, err = scanFlat(config, allowedExts)
	}

	elapsed := time.Since(startTime)

	if err != nil {
		// Clean the line in case we errored while scanning
		if showLive {
			fmt.Print(ClearLine + "\r")
		}
		printError(fmt.Sprintf("Error scanning: %v", err))
		os.Exit(1)
	}

	// 5. Print Summary
	if showLive {
		//  Clear the "Scanning..." line before printing summary
		fmt.Print(ClearLine + "\r")
	}

	printSummary(scannedCount, len(results), elapsed)

	// 6. Output Results
	if len(results) == 0 {
		printWarning("No files found matching your criteria.")
		return
	}

	handleOutput(results, config)
}

func parseFlags() Config {
	var c Config
	flag.StringVar(&c.Dir, "dir", ".", "Target directory path")
	flag.IntVar(&c.Day, "dt", 0, "Day (1-31)")
	flag.IntVar(&c.Month, "m", 0, "Month (1-12)")
	flag.IntVar(&c.Year, "y", 0, "Year (e.g. 2024)")
	flag.StringVar(&c.AllDate, "all", "", "Complete date 'DD/M/YYYY' (e.g. 24/1/2026)")
	flag.BoolVar(&c.Recursive, "r", false, "Enable recursive DFS scan")
	flag.StringVar(&c.Extensions, "ex", "", "Comma separated extensions (e.g. go,py,txt)")
	flag.StringVar(&c.OutputFormat, "o", "tabular", "Output format: tabular, json, md")
	flag.Parse()
	return c
}

// scanFlat: Only looks at the top directory
func scanFlat(c Config, exts map[string]bool) ([]FileResult, error) {
	var results []FileResult
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		scannedCount++

		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if showLive {
			printLiveScanning(filepath.Join(c.Dir, entry.Name()))
		}

		if isMatch(info, c, exts) {
			matchCount++
			if showLive {
				printLiveMatch(filepath.Join(c.Dir, entry.Name()))
			}
			results = append(results, buildResult(c.Dir, info))
		}
	}
	return results, nil
}

// scanRecursive: Uses filepath.WalkDir for efficient DFS traversal
func scanRecursive(c Config, exts map[string]bool) ([]FileResult, error) {
	var results []FileResult

	err := filepath.WalkDir(c.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		// Show directory scanning (Will be overwritten by file scanning)
		if d.IsDir() {
			if showLive {
				printLiveDirectory(path)
			}
			return nil
		}

		scannedCount++

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Update UI every 50 files to prevent flickering/slowing down
		if showLive && scannedCount%50 == 0 {
			printLiveScanning(path)
		}

		if isMatch(info, c, exts) {
			matchCount++
			if showLive {
				printLiveMatch(path)
			}
			results = append(results, buildResult(filepath.Dir(path), info))
		}

		return nil
	})

	return results, err
}

func buildResult(dir string, info os.FileInfo) FileResult {
	return FileResult{
		Name:         info.Name(),
		Path:         filepath.Join(dir, info.Name()),
		LastModified: info.ModTime().Format("2006-01-02 15:04:05"),
		ModTimeRaw:   info.ModTime(),
		Size:         info.Size(),
	}
}

// isMatch: The brain of the filter logic
func isMatch(info os.FileInfo, c Config, exts map[string]bool) bool {
	// 1. Extension Check
	if len(exts) > 0 {
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !exts[ext] {
			return false
		}
	}

	// 2. Date Check
	y, m, d := info.ModTime().Date()

	// Logic: -all flag takes priority
	if c.AllDate != "" {
		parts := strings.Split(c.AllDate, "/")
		if len(parts) == 3 {
			reqD, _ := strconv.Atoi(parts[0])
			reqM, _ := strconv.Atoi(parts[1])
			reqY, _ := strconv.Atoi(parts[2])

			if reqD != d || reqM != int(m) || reqY != y {
				return false
			}
			return true
		}
	}

	// Partial Logic
	if c.Year != 0 && c.Year != y {
		return false
	}
	if c.Month != 0 && c.Month != int(m) {
		return false
	}
	if c.Day != 0 && c.Day != d {
		return false
	}

	return true
}

// handleOutput routes to specific formatters
func handleOutput(results []FileResult, c Config) {
	switch strings.ToLower(c.OutputFormat) {
	case "json":
		saveJSON(results, c.Extensions)
	case "md":
		saveMarkdown(results, c.Extensions)
	default:
		printTabular(results)
	}
}

func printTabular(results []FileResult) {
	fmt.Println()
	printSuccess("Search Results:")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "%s%sDATE\tSIZE\tFILE\tPATH%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Fprintf(w, "%s----\t----\t----\t----%s\n", ColorDim, ColorReset)

	for _, r := range results {
		sizeStr := formatSize(r.Size)
		fmt.Fprintf(w, "%s%s%s\t%s%s%s\t%s\t%s%s%s\n",
			ColorYellow, r.LastModified, ColorReset,
			ColorGreen, sizeStr, ColorReset,
			r.Name,
			ColorDim, r.Path, ColorReset)
	}
	w.Flush()
	fmt.Println()
}

func saveJSON(results []FileResult, exts string) {
	filename := generateFilename("output", exts, "json")
	file, err := os.Create(filename)
	if err != nil {
		printError(fmt.Sprintf("Error creating file: %v", err))
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(results)

	printSuccess(fmt.Sprintf("JSON saved to: %s", filename))
}

func saveMarkdown(results []FileResult, exts string) {
	filename := generateFilename("output", exts, "md")
	file, err := os.Create(filename)
	if err != nil {
		printError(fmt.Sprintf("Error creating file: %v", err))
		return
	}
	defer file.Close()

	fmt.Fprintln(file, "# 🔍 File Search Results")
	fmt.Fprintf(file, "**Generated:** %s\n\n", time.Now().Format(time.RFC1123))
	fmt.Fprintf(file, "**Total Files Found:** %d\n\n", len(results))
	fmt.Fprintln(file, "| Last Modified | Size | File Name | Full Path |")
	fmt.Fprintln(file, "|---|---|---|---|")

	for _, r := range results {
		safePath := strings.ReplaceAll(r.Path, "|", "\\|")
		sizeStr := formatSize(r.Size)
		fmt.Fprintf(file, "| %s | %s | %s | %s |\n", r.LastModified, sizeStr, r.Name, safePath)
	}

	printSuccess(fmt.Sprintf("Markdown saved to: %s", filename))
}

func generateFilename(prefix, exts, suffix string) string {
	extPart := "all"
	if exts != "" {
		extPart = strings.ReplaceAll(exts, ",", "_")
	}
	return fmt.Sprintf("%s_%s.%s", prefix, extPart, suffix)
}

func formatSize(bytes int64) string {
	const kb = 1024
	const mb = kb * 1024
	const gb = mb * 1024

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ============ PRINTING UTILITIES ============

func printHeader() {
	fmt.Printf("\n%s%s", ColorBold, ColorCyan)
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║     FILE SEARCH CLI TOOL v1.1             ║")
	fmt.Println("╔═══════════════════════════════════════════╝")
	fmt.Printf("%s\n", ColorReset)
}

func printInfo(msg string) {
	fmt.Printf("%s🔍 %s%s\n", ColorBlue, msg, ColorReset)
}

func printSuccess(msg string) {
	fmt.Printf("%s✅ %s%s\n", ColorGreen, msg, ColorReset)
}

func printWarning(msg string) {
	fmt.Printf("%s⚠️  %s%s\n", ColorYellow, msg, ColorReset)
}

func printError(msg string) {
	fmt.Printf("%s❌ Error: %s%s\n", ColorRed, msg, ColorReset)
}

// Clears the line before printing
func printLiveDirectory(path string) {
	// Truncate long paths
	displayPath := path
	if len(path) > 60 {
		displayPath = "..." + path[len(path)-57:]
	}
	// ClearLine (\033[2K) ensures no garbage is left from previous longer lines
	fmt.Printf("%s\r%s📂 Scanning: %-60s%s", ClearLine, ColorCyan, displayPath, ColorReset)
}

// Clears the line before printing
func printLiveScanning(path string) {
	displayPath := filepath.Base(path)
	if len(displayPath) > 40 {
		displayPath = displayPath[:37] + "..."
	}
	// ClearLine (\033[2K) prevents the "ghost" text overlap
	fmt.Printf("%s\r%s🔎 Checking: %-40s [Scanned: %d]%s",
		ClearLine, ColorDim, displayPath, scannedCount, ColorReset)
}

// Prints with a NEWLINE (\n) so matches don't get overwritten
func printLiveMatch(path string) {
	displayPath := filepath.Base(path)
	if len(displayPath) > 40 {
		displayPath = displayPath[:37] + "..."
	}

	// 1. Clear the "Checking..." line
	// 2. Print the match
	// 3. Print newline so it sticks
	fmt.Printf("%s\r%s✓ Match: %-40s [Found: %d]%s\n",
		ClearLine, ColorGreen, displayPath, matchCount, ColorReset)
}

func printSummary(scanned, found int, elapsed time.Duration) {
	fmt.Printf("\n%s%s", ColorBold, ColorWhite)
	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Printf("║  SCAN COMPLETE                            ║\n")
	fmt.Println("╠═══════════════════════════════════════════╣")
	fmt.Printf("║  Files Scanned: %-26d ║\n", scanned)
	fmt.Printf("║  Matches Found: %-26d ║\n", found)
	fmt.Printf("║  Time Taken:    %-26s ║\n", elapsed.Round(time.Millisecond))
	fmt.Println("╚═══════════════════════════════════════════╝")
	fmt.Printf("%s\n", ColorReset)
}

func printDateFilter(c Config) {
	if c.AllDate != "" {
		printInfo(fmt.Sprintf("Date Filter: Exact match for %s", c.AllDate))
	} else {
		var parts []string
		if c.Day != 0 {
			parts = append(parts, fmt.Sprintf("Day=%d", c.Day))
		}
		if c.Month != 0 {
			parts = append(parts, fmt.Sprintf("Month=%d", c.Month))
		}
		if c.Year != 0 {
			parts = append(parts, fmt.Sprintf("Year=%d", c.Year))
		}
		if len(parts) > 0 {
			printInfo(fmt.Sprintf("Date Filter: %s", strings.Join(parts, ", ")))
		}
	}
}
