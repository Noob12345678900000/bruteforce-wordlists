package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	entriesPerFile = 2_000_000 // 2 million combinations per file
	batchSize      = 250_000   // Optimized batch for smooth progress + speed
	maxLength      = 4         // Lengths 1 to 5
	commitEvery    = 20        // Git commit & push every 10 files
)

var (
	// Charset: a-z, A-Z, 0-9, _, .
	charset = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.")
	N       = len(charset)
	pow     = [6]int64{1, 1, 1, 1, 1, 1} // N^0 to N^5
	cum     = [6]int64{0, 0, 0, 0, 0, 0} // Cumulative totals up to length l
	total   int64
)

func initTotals() {
	p := int64(1)
	for l := 1; l <= maxLength; l++ {
		p *= int64(N)
		pow[l] = p
		cum[l] = cum[l-1] + p
	}
	total = cum[maxLength]
}

func getCombo(pos int64) string {
	// Find length
	var L int
	for l := 1; l <= maxLength; l++ {
		if pos < cum[l] {
			L = l
			break
		}
	}
	offset := pos - cum[L-1]

	// Build string efficiently
	s := make([]byte, L)
	for j := L - 1; j >= 0; j-- {
		s[j] = charset[offset%int64(N)]
		offset /= int64(N)
	}
	return string(s)
}

func gitCommitAndPush(filesCompleted int) {
	fmt.Printf("\n🔄 Committing and pushing progress (%d files completed)...\n", filesCompleted)

	commands := []struct {
		name string
		args []string
	}{
		{"git add", []string{"add", "."}},
		{"git commit", []string{"commit", "-m", fmt.Sprintf("Wordlist progress: added files up to combos_%06d.txt (%d files)", filesCompleted, filesCompleted)}},
		{"git push", []string{"push", "origin", "main"}},
	}

	for _, cmd := range commands {
		c := exec.Command("git", cmd.args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Printf("⚠️  %s failed: %v\n", cmd.name, err)
			return // Stop on failure (e.g. auth or network issue)
		}
	}
	fmt.Println("✅ Successfully committed and pushed!\n")
}

func main() {
	initTotals()

	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Alphanumeric + _ . Wordlist Generator         ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Printf("Charset   : a-z A-Z 0-9 _ .  (%d characters)\n", N)
	fmt.Printf("Lengths   : 1 to %d characters\n", maxLength)
	fmt.Printf("Total     : %,d combinations (~%.3f billion)\n", total, float64(total)/1e9)
	fmt.Printf("Per file  : %,d entries\n", entriesPerFile)
	fmt.Printf("Files     : ~%d total\n", (total+entriesPerFile-1)/entriesPerFile)
	fmt.Println("────────────────────────────────────────────────────────────\n")

	stateFile := "state.txt"
	var currentPos int64

	if data, err := os.ReadFile(stateFile); err == nil {
		currentPos, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		currentPos++
		donePercent := float64(currentPos-1) / float64(total) * 100
		fmt.Printf("📂 Resuming from position %,d (%.4f%% complete)\n\n", currentPos-1, donePercent)
	} else {
		fmt.Println("🚀 Starting fresh generation...\n")
	}

	startTime := time.Now()
	lastUpdate := startTime
	var generatedSinceLast int64

	filesCompleted := int(currentPos / entriesPerFile)

	stdoutWriter := bufio.NewWriter(os.Stdout)

	for currentPos < total {
		fileNum := int(currentPos/entriesPerFile) + 1
		fileName := fmt.Sprintf("combos_%06d.txt", fileNum)

		file, err := os.Create(fileName)
		if err != nil {
			panic(err)
		}
		writer := bufio.NewWriter(file)

		remainingInFile := entriesPerFile
		if currentPos+int64(entriesPerFile) > total {
			remainingInFile = int(total - currentPos)
		}

		written := 0
		for written < remainingInFile {
			batchEnd := currentPos + batchSize
			if batchEnd > currentPos+int64(remainingInFile-written) {
				batchEnd = currentPos + int64(remainingInFile-written)
			}
			if batchEnd > total {
				batchEnd = total
			}

			for pos := currentPos; pos < batchEnd; pos++ {
				writer.WriteString(getCombo(pos) + "\n")
			}

			count := batchEnd - currentPos
			generatedSinceLast += count
			currentPos += count
			written += int(count)

			// Progress update
			now := time.Now()
			if now.Sub(lastUpdate).Seconds() >= 0.15 {
				elapsed := now.Sub(lastUpdate).Seconds()
				speed := float64(generatedSinceLast) / elapsed
				percent := float64(currentPos) / float64(total) * 100

				barFilled := int(percent / 2)
				if barFilled > 50 {
					barFilled = 50
				}
				bar := strings.Repeat("█", barFilled) + strings.Repeat("░", 50-barFilled)

				etaSeconds := float64(total-currentPos) / speed
				eta := time.Duration(etaSeconds) * time.Second
				etaStr := fmt.Sprintf("%02dh%02dm%02ds", int(eta.Hours()), int(eta.Minutes())%60, int(eta.Seconds())%60)

				fmt.Fprintf(stdoutWriter,
					"\r🔧 File %06d │ %s %.4f%% │ %,10d / %,10d │ Speed: %8.0f/s │ ETA: %s",
					fileNum, bar, percent, currentPos, total, speed, etaStr)

				stdoutWriter.Flush()
				generatedSinceLast = 0
				lastUpdate = now
			}
		}

		writer.Flush()
		file.Close()

		// Save progress
		os.WriteFile(stateFile, []byte(strconv.FormatInt(currentPos-1, 10)), 0644)

		filesCompleted++
		fmt.Printf("\n✅ Completed: %s (%,d entries) — Total files: %d\n", fileName, written, filesCompleted)

		// Auto git commit every N files
		if filesCompleted%commitEvery == 0 {
			gitCommitAndPush(filesCompleted)
		}
	}

	// Final commit if needed
	if filesCompleted%commitEvery != 0 {
		gitCommitAndPush(filesCompleted)
	}

	totalTime := time.Since(startTime)
	avgSpeed := float64(total) / totalTime.Seconds()

	fmt.Println("\n╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     🎉 GENERATION COMPLETE!                ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Printf("Total combinations : %,d\n", total)
	fmt.Printf("Time taken         : %v\n", totalTime.Round(time.Second))
	fmt.Printf("Average speed      : %.0f combinations/sec\n", avgSpeed)
	fmt.Printf("Total files        : %d\n", filesCompleted)
	fmt.Println("All files saved as combos_XXXXXX.txt")
	fmt.Println("Progress backed up via git every 10 files.\n")
}
