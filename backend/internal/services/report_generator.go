package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/rubix-simulator/backend/internal/config"
	"github.com/rubix-simulator/backend/internal/models"
)

type ReportGenerator struct {
	config      *config.Config
	reportsPath string
}

func NewReportGenerator(cfg *config.Config) *ReportGenerator {
	reportsPath := filepath.Join(".", "reports")
	os.MkdirAll(reportsPath, 0o755)

	return &ReportGenerator{
		config:      cfg,
		reportsPath: reportsPath,
	}
}

// formatDuration converts a time.Duration to human-readable format (e.g., "1m10s", "45s", "2m30s")
func formatDuration(d time.Duration) string {
	if d < time.Second {
		// For sub-second durations, show milliseconds
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	
	if minutes > 0 {
		if seconds > 0 {
			return fmt.Sprintf("%dm%ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", seconds)
}

func (rg *ReportGenerator) GeneratePDF(report *models.SimulationReport) (string, error) {
	filename := fmt.Sprintf("simulation-%s.pdf", report.SimulationID)
	filepath := filepath.Join(rg.reportsPath, filename)

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 10)
	pdf.AddPage()

	rg.addHeader(pdf, report)
	rg.addSummary(pdf, report)
	rg.addTokenAnalysis(pdf, report) // Changed from addNodeBreakdown
	rg.addTransactionDetails(pdf, report)
	rg.addCharts(pdf, report)

	if err := pdf.OutputFileAndClose(filepath); err != nil {
		return "", fmt.Errorf("failed to save PDF: %v", err)
	}

	log.Printf("Report generated: %s", filepath)
	return filename, nil
}

func (rg *ReportGenerator) addHeader(pdf *fpdf.Fpdf, report *models.SimulationReport) {
	pdf.SetFont("Arial", "B", 20)
	pdf.CellFormat(0, 15, "Rubix Network Simulation Report", "", 1, "C", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 8, fmt.Sprintf("Simulation ID: %s", report.SimulationID), "", 1, "C", false, 0, "")
	pdf.CellFormat(0, 8, fmt.Sprintf("Generated: %s", report.CreatedAt.Format("2006-01-02 15:04:05")), "", 1, "C", false, 0, "")
	pdf.Ln(10)
}

func (rg *ReportGenerator) addSummary(pdf *fpdf.Fpdf, report *models.SimulationReport) {
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "Summary", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)

	// Convert average latency from milliseconds to Duration
	avgLatencyDuration := time.Duration(report.AverageLatency) * time.Millisecond
	
	summaryData := [][]string{
		{"Parameter", "Value"},
		{"Total Nodes", fmt.Sprintf("%d", len(report.Nodes))},
		{"Total Transactions", fmt.Sprintf("%d", report.TotalTransactions)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", report.SuccessCount,
			float64(report.SuccessCount)/float64(report.TotalTransactions)*100)},
		{"Failed", fmt.Sprintf("%d (%.1f%%)", report.FailureCount,
			float64(report.FailureCount)/float64(report.TotalTransactions)*100)},
		{"Average Latency", formatDuration(avgLatencyDuration)},
		{"Min Latency", formatDuration(report.MinLatency)},
		{"Max Latency", formatDuration(report.MaxLatency)},
		{"Total Tokens Transferred", fmt.Sprintf("%.2f", report.TotalTokensTransferred)},
		{"Total Execution Time", formatDuration(report.TotalTime)},
	}

	rg.addTable(pdf, summaryData, []float64{60, 100})
	pdf.Ln(10)
}

// addTokenAnalysis adds token transfer performance analysis grouped by token ranges
func (rg *ReportGenerator) addTokenAnalysis(pdf *fpdf.Fpdf, report *models.SimulationReport) {
	if len(report.Transactions) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "Token Transfer Performance Analysis", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)

	// Define token ranges (1-10 tokens, 1 token intervals)
	ranges := []struct {
		min, max float64
		label    string
	}{
		{1.0, 2.0, "1.0-2.0"},
		{2.0, 3.0, "2.0-3.0"},
		{3.0, 4.0, "3.0-4.0"},
		{4.0, 5.0, "4.0-5.0"},
		{5.0, 6.0, "5.0-6.0"},
		{6.0, 7.0, "6.0-7.0"},
		{7.0, 8.0, "7.0-8.0"},
		{8.0, 9.0, "8.0-9.0"},
		{9.0, 10.0, "9.0-10.0"},
	}

	// Prepare data for table
	analysisData := [][]string{
		{"Token Range", "Transactions", "Avg Time(ms)", "Min Time", "Max Time", "Success Rate"},
	}

	// Analyze transactions by token range
	for _, r := range ranges {
		var transactions []models.Transaction
		var totalTime time.Duration
		var minTime time.Duration = time.Hour * 24 // Initialize to very high value
		var maxTime time.Duration
		var successCount int

		// Collect transactions in this range
		for _, tx := range report.Transactions {
			if tx.TokenAmount >= r.min && tx.TokenAmount < r.max {
				transactions = append(transactions, tx)
				totalTime += tx.TimeTaken

				if tx.TimeTaken < minTime {
					minTime = tx.TimeTaken
				}
				if tx.TimeTaken > maxTime {
					maxTime = tx.TimeTaken
				}

				if tx.Status == "success" {
					successCount++
				}
			}
		}

		// Only add row if there are transactions in this range
		if len(transactions) > 0 {
			avgTime := totalTime / time.Duration(len(transactions))
			successRate := float64(successCount) / float64(len(transactions)) * 100

			analysisData = append(analysisData, []string{
				r.label,
				fmt.Sprintf("%d", len(transactions)),
				formatDuration(avgTime),
				formatDuration(minTime),
				formatDuration(maxTime),
				fmt.Sprintf("%.1f%%", successRate),
			})
		}
	}

	// Only render if we have data
	if len(analysisData) > 1 {
		rg.addTable(pdf, analysisData, []float64{30, 30, 30, 30, 30, 30})
		pdf.Ln(10)
	}
}

// Keep old function for backward compatibility but it now calls the new one
func (rg *ReportGenerator) addNodeBreakdown(pdf *fpdf.Fpdf, report *models.SimulationReport) {
	if len(report.NodeBreakdown) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "Node Performance Breakdown", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)

	nodeData := [][]string{
		{"Node ID", "Transactions", "Success", "Failed", "Avg Latency", "Tokens"},
	}

	for _, node := range report.NodeBreakdown {
		// Safely truncate NodeID to prevent slice bounds error
		nodeIDDisplay := node.NodeID
		if len(nodeIDDisplay) > 8 {
			nodeIDDisplay = nodeIDDisplay[:8]
		}

		nodeData = append(nodeData, []string{
			nodeIDDisplay,
			fmt.Sprintf("%d", node.TransactionsHandled),
			fmt.Sprintf("%d", node.SuccessfulTransactions),
			fmt.Sprintf("%d", node.FailedTransactions),
			formatDuration(node.AverageLatency),
			fmt.Sprintf("%.2f", node.TotalTokensTransferred),
		})
	}

	rg.addTable(pdf, nodeData, []float64{30, 30, 25, 25, 35, 35})
	pdf.Ln(10)
}

func (rg *ReportGenerator) addTransactionDetails(pdf *fpdf.Fpdf, report *models.SimulationReport) {
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "Transaction Log (Sorted by Token Amount)", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 8)

	// Create a copy of transactions to sort (to avoid modifying original)
	sortedTransactions := make([]models.Transaction, len(report.Transactions))
	copy(sortedTransactions, report.Transactions)

	// Sort by token amount (ascending)
	sort.Slice(sortedTransactions, func(i, j int) bool {
		return sortedTransactions[i].TokenAmount < sortedTransactions[j].TokenAmount
	})

	maxTransactions := 50
	if len(sortedTransactions) < maxTransactions {
		maxTransactions = len(sortedTransactions)
	}

	txData := [][]string{
		{"TX ID", "Tokens", "Time", "Status", "Node"},
	}

	for i := 0; i < maxTransactions; i++ {
		tx := sortedTransactions[i]

		// Safely truncate IDs to prevent slice bounds error
		txIDDisplay := tx.ID
		if len(txIDDisplay) > 8 {
			txIDDisplay = txIDDisplay[:8]
		}

		nodeIDDisplay := tx.NodeID
		if len(nodeIDDisplay) > 8 {
			nodeIDDisplay = nodeIDDisplay[:8]
		}

		txData = append(txData, []string{
			txIDDisplay,
			fmt.Sprintf("%.3f", tx.TokenAmount), // Format as float with 3 decimal places
			formatDuration(tx.TimeTaken),
			tx.Status,
			nodeIDDisplay,
		})
	}

	rg.addTable(pdf, txData, []float64{30, 25, 25, 30, 30})

	if len(sortedTransactions) > maxTransactions {
		pdf.SetFont("Arial", "I", 8)
		pdf.CellFormat(0, 10, fmt.Sprintf("... and %d more transactions (sorted by token amount)", len(sortedTransactions)-maxTransactions), "", 1, "C", false, 0, "")
	}
}

func (rg *ReportGenerator) addCharts(pdf *fpdf.Fpdf, report *models.SimulationReport) {
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 10, "Performance Charts", "", 1, "L", false, 0, "")

	// Add the new token vs time chart as the primary chart
	rg.drawTokenVsTimeChart(pdf, report, 30, 40)
	
	// Keep existing charts but make them smaller
	rg.drawSuccessRatePieChart(pdf, report, 30, 140)
	rg.drawLatencyHistogram(pdf, report, 110, 140)
	rg.drawNodeLoadChart(pdf, report, 30, 210)
}

// drawTokenVsTimeChart draws a scatter plot showing relationship between token amount and transaction time
func (rg *ReportGenerator) drawTokenVsTimeChart(pdf *fpdf.Fpdf, report *models.SimulationReport, x, y float64) {
	if len(report.Transactions) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 12)
	pdf.SetXY(x, y-10)
	pdf.CellFormat(150, 10, "Token Amount vs Transaction Time", "", 0, "C", false, 0, "")
	
	// Chart dimensions
	chartWidth := float64(150)
	chartHeight := float64(80)
	chartX := x
	chartY := y
	
	// Draw axes
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(chartX, chartY+chartHeight, chartX+chartWidth, chartY+chartHeight) // X-axis
	pdf.Line(chartX, chartY, chartX, chartY+chartHeight) // Y-axis
	
	// Find min/max values for scaling
	minTokens, maxTokens := 10.0, 0.0
	minTime, maxTime := float64(1000000), float64(0)
	
	for _, tx := range report.Transactions {
		if tx.Status == "success" { // Only plot successful transactions
			if tx.TokenAmount < minTokens {
				minTokens = tx.TokenAmount
			}
			if tx.TokenAmount > maxTokens {
				maxTokens = tx.TokenAmount
			}
			
			timeMs := float64(tx.TimeTaken.Milliseconds())
			if timeMs < minTime && timeMs > 0 {
				minTime = timeMs
			}
			if timeMs > maxTime {
				maxTime = timeMs
			}
		}
	}
	
	// Add some padding to the ranges
	tokenRange := maxTokens - minTokens
	if tokenRange == 0 {
		tokenRange = 1
	}
	timeRange := maxTime - minTime
	if timeRange == 0 {
		timeRange = 1000
	}
	
	// Draw grid lines and labels
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetFont("Arial", "", 8)
	
	// X-axis labels (token amounts)
	for i := 0; i <= 5; i++ {
		xPos := chartX + (float64(i) * chartWidth / 5)
		pdf.Line(xPos, chartY, xPos, chartY+chartHeight)
		
		tokenValue := minTokens + (float64(i) * tokenRange / 5)
		pdf.SetXY(xPos-10, chartY+chartHeight+2)
		pdf.CellFormat(20, 5, fmt.Sprintf("%.1f", tokenValue), "", 0, "C", false, 0, "")
	}
	
	// Y-axis labels (time in human-readable format)
	for i := 0; i <= 4; i++ {
		yPos := chartY + chartHeight - (float64(i) * chartHeight / 4)
		pdf.Line(chartX, yPos, chartX+chartWidth, yPos)
		
		timeValue := minTime + (float64(i) * timeRange / 4)
		timeDuration := time.Duration(timeValue) * time.Millisecond
		pdf.SetXY(chartX-25, yPos-2)
		pdf.CellFormat(20, 5, formatDuration(timeDuration), "", 0, "R", false, 0, "")
	}
	
	// Plot data points
	for _, tx := range report.Transactions {
		if tx.Status == "success" {
			// Calculate position
			xPos := chartX + ((tx.TokenAmount - minTokens) / tokenRange * chartWidth)
			timeMs := float64(tx.TimeTaken.Milliseconds())
			yPos := chartY + chartHeight - ((timeMs - minTime) / timeRange * chartHeight)
			
			// Draw point (circle)
			pdf.SetFillColor(33, 150, 243) // Blue for success
			pdf.Circle(xPos, yPos, 1.5, "F")
		}
	}
	
	// Add axis labels
	pdf.SetFont("Arial", "", 9)
	pdf.SetXY(chartX + chartWidth/2 - 20, chartY + chartHeight + 10)
	pdf.CellFormat(40, 5, "Token Amount (RBT)", "", 0, "C", false, 0, "")
	
	// Y-axis label (rotated text would be ideal but fpdf has limitations)
	pdf.SetXY(chartX - 30, chartY + chartHeight/2 - 5)
	pdf.CellFormat(20, 5, "Time", "", 0, "C", false, 0, "")
	
	// Add insights below the chart
	pdf.SetFont("Arial", "I", 8)
	pdf.SetXY(x, y + chartHeight + 20)
	
	// Calculate correlation or trend
	avgSmallTime := float64(0)
	avgLargeTime := float64(0)
	smallCount := 0
	largeCount := 0
	
	midToken := (minTokens + maxTokens) / 2
	
	for _, tx := range report.Transactions {
		if tx.Status == "success" {
			timeMs := float64(tx.TimeTaken.Milliseconds())
			if tx.TokenAmount < midToken {
				avgSmallTime += timeMs
				smallCount++
			} else {
				avgLargeTime += timeMs
				largeCount++
			}
		}
	}
	
	if smallCount > 0 {
		avgSmallTime /= float64(smallCount)
	}
	if largeCount > 0 {
		avgLargeTime /= float64(largeCount)
	}
	
	avgSmallDuration := time.Duration(avgSmallTime) * time.Millisecond
	avgLargeDuration := time.Duration(avgLargeTime) * time.Millisecond
	
	insight := fmt.Sprintf("Avg time for <%.1f RBT: %s | Avg time for >%.1f RBT: %s",
		midToken, formatDuration(avgSmallDuration), midToken, formatDuration(avgLargeDuration))
	pdf.CellFormat(150, 5, insight, "", 0, "C", false, 0, "")
}

func (rg *ReportGenerator) drawSuccessRatePieChart(pdf *fpdf.Fpdf, report *models.SimulationReport, x, y float64) {
	pdf.SetFont("Arial", "B", 12)
	pdf.SetXY(x, y-10)
	pdf.CellFormat(60, 10, "Success Rate", "", 0, "C", false, 0, "")

	centerX, centerY := x+30, y+30
	radius := 25.0

	successAngle := 360.0 * float64(report.SuccessCount) / float64(report.TotalTransactions)

	pdf.SetFillColor(76, 175, 80)
	pdf.Circle(centerX, centerY, radius, "F")

	pdf.SetFillColor(244, 67, 54)
	if report.FailureCount > 0 {
		// Simple rectangle representation for failure portion
		failureHeight := radius * 2 * float64(report.FailureCount) / float64(report.TotalTransactions)
		pdf.Rect(centerX+radius+5, centerY-radius+successAngle/360*radius*2, 10, failureHeight, "F")
	}

	pdf.SetFont("Arial", "", 8)
	pdf.SetXY(x, y+60)
	pdf.SetTextColor(76, 175, 80)
	pdf.CellFormat(30, 5, fmt.Sprintf("Success: %.1f%%", float64(report.SuccessCount)/float64(report.TotalTransactions)*100), "", 0, "L", false, 0, "")
	pdf.SetTextColor(244, 67, 54)
	pdf.CellFormat(30, 5, fmt.Sprintf("Failed: %.1f%%", float64(report.FailureCount)/float64(report.TotalTransactions)*100), "", 0, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

func (rg *ReportGenerator) drawLatencyHistogram(pdf *fpdf.Fpdf, report *models.SimulationReport, x, y float64) {
	pdf.SetFont("Arial", "B", 12)
	pdf.SetXY(x, y-10)
	pdf.CellFormat(60, 10, "Latency Distribution", "", 0, "C", false, 0, "")

	bins := make(map[string]int)
	for _, tx := range report.Transactions {
		latency := tx.TimeTaken.Milliseconds()
		var bin string
		switch {
		case latency < 100:
			bin = "<100ms"
		case latency < 200:
			bin = "100-200ms"
		case latency < 500:
			bin = "200-500ms"
		case latency < 1000:
			bin = "500-1000ms"
		default:
			bin = ">1000ms"
		}
		bins[bin]++
	}

	maxCount := 0
	for _, count := range bins {
		if count > maxCount {
			maxCount = count
		}
	}

	barWidth := 10.0
	maxHeight := 40.0
	startX := x

	binOrder := []string{"<100ms", "100-200ms", "200-500ms", "500-1000ms", ">1000ms"}

	for i, bin := range binOrder {
		count := bins[bin]
		if maxCount > 0 {
			height := (float64(count) / float64(maxCount)) * maxHeight
			pdf.SetFillColor(33, 150, 243)
			pdf.Rect(startX+float64(i)*barWidth, y+50-height, barWidth-1, height, "F")

			pdf.SetFont("Arial", "", 6)
			// Simple text label below bar
			pdf.SetXY(startX+float64(i)*barWidth-5, y+55)
			pdf.SetFont("Arial", "", 6)
			pdf.CellFormat(barWidth+10, 3, bin, "", 0, "C", false, 0, "")
		}
	}
}

func (rg *ReportGenerator) drawNodeLoadChart(pdf *fpdf.Fpdf, report *models.SimulationReport, x, y float64) {
	if len(report.NodeBreakdown) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 12)
	pdf.SetXY(x, y-10)
	pdf.CellFormat(150, 10, "Node Load Distribution", "", 0, "C", false, 0, "")

	barWidth := 140.0 / float64(len(report.NodeBreakdown))
	maxTransactions := 0

	for _, node := range report.NodeBreakdown {
		if node.TransactionsHandled > maxTransactions {
			maxTransactions = node.TransactionsHandled
		}
	}

	if maxTransactions == 0 {
		return
	}

	maxHeight := 30.0

	for i, node := range report.NodeBreakdown {
		height := (float64(node.TransactionsHandled) / float64(maxTransactions)) * maxHeight

		successHeight := height * (float64(node.SuccessfulTransactions) / float64(node.TransactionsHandled))
		failHeight := height - successHeight

		nodeX := x + float64(i)*barWidth

		pdf.SetFillColor(76, 175, 80)
		pdf.Rect(nodeX, y+30-successHeight, barWidth-2, successHeight, "F")

		pdf.SetFillColor(244, 67, 54)
		pdf.Rect(nodeX, y+30-height, barWidth-2, failHeight, "F")

		pdf.SetFont("Arial", "", 7)
		pdf.SetXY(nodeX, y+32)
		// Safely truncate NodeID for display
		nodeLabel := node.NodeID
		if len(nodeLabel) > 6 {
			nodeLabel = nodeLabel[:6]
		}
		pdf.CellFormat(barWidth, 5, nodeLabel, "", 0, "C", false, 0, "")
	}
}

func (rg *ReportGenerator) addTable(pdf *fpdf.Fpdf, data [][]string, widths []float64) {
	for i, row := range data {
		if i == 0 {
			pdf.SetFont("Arial", "B", 10)
			pdf.SetFillColor(240, 240, 240)
		} else {
			pdf.SetFont("Arial", "", 10)
			pdf.SetFillColor(255, 255, 255)
		}

		for j, cell := range row {
			width := widths[j]
			pdf.CellFormat(width, 8, cell, "1", 0, "C", i == 0, 0, "")
		}
		pdf.Ln(-1)
	}
}

func (rg *ReportGenerator) GetReportPath(filename string) string {
	return filepath.Join(rg.reportsPath, filename)
}

func (rg *ReportGenerator) ListReports() ([]models.ReportInfo, error) {
	files, err := os.ReadDir(rg.reportsPath)
	if err != nil {
		return nil, err
	}

	var reports []models.ReportInfo
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".pdf" {
			info, err := file.Info()
			if err != nil {
				continue
			}

			reports = append(reports, models.ReportInfo{
				ID:        file.Name()[:len(file.Name())-4],
				Filename:  file.Name(),
				CreatedAt: info.ModTime(),
				Size:      info.Size(),
			})
		}
	}

	return reports, nil
}
