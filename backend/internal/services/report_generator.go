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
	avgTransactionTimeDuration := time.Duration(report.AverageTransactionTime) * time.Millisecond
	
	summaryData := [][]string{
		{"Parameter", "Value"},
		{"Total Nodes", fmt.Sprintf("%d", len(report.Nodes))},
		{"Total Transactions", fmt.Sprintf("%d", report.TotalTransactions)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", report.SuccessCount,
			float64(report.SuccessCount)/float64(report.TotalTransactions)*100)},
		{"Failed", fmt.Sprintf("%d (%.1f%%)", report.FailureCount,
			float64(report.FailureCount)/float64(report.TotalTransactions)*100)},
		{"Average Transaction Time", formatDuration(avgTransactionTimeDuration)},
		{"Min Transaction Time", formatDuration(report.MinTransactionTime)},
		{"Max Transaction Time", formatDuration(report.MaxTransactionTime)},
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
		{"Node ID", "Transactions", "Success", "Failed", "Avg Transaction Time", "Tokens"},
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
			formatDuration(node.AverageTransactionTime),
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
	pdf.CellFormat(0, 10, "Performance Chart", "", 1, "L", false, 0, "")

	rg.drawAvgTimeVsTokenRangeChart(pdf, report, 30, 40)
}

func (rg *ReportGenerator) drawAvgTimeVsTokenRangeChart(pdf *fpdf.Fpdf, report *models.SimulationReport, x, y float64) {
	if len(report.Transactions) == 0 {
		return
	}

	pdf.SetFont("Arial", "B", 12)
	pdf.SetXY(x, y-10)
	pdf.CellFormat(150, 10, "Average Time vs. Token Range", "", 0, "C", false, 0, "")

	// Define token ranges
	ranges := []struct {
		min, max float64
		label    string
	}{
		{1, 1, "1"},
		{2, 2, "2"},
		{3, 3, "3"},
		{4, 4, "4"},
		{5, 5, "5"},
		{6, 6, "6"},
		{7, 7, "7"},
		{8, 8, "8"},
		{9, 9, "9"},
		{10, 10, "10"},
	}

	// Calculate average time for each token range
	rangeAvgTimes := make(map[string]float64)
	rangeCounts := make(map[string]int)
	for _, tx := range report.Transactions {
		if tx.Status == "success" {
			for _, r := range ranges {
				if tx.TokenAmount >= r.min && tx.TokenAmount <= r.max {
					rangeAvgTimes[r.label] += float64(tx.TimeTaken.Milliseconds())
					rangeCounts[r.label]++
					break
				}
			}
		}
	}

	// Chart dimensions
	chartWidth := float64(150)
	chartHeight := float64(80)
	chartX := x
	chartY := y

	// Draw axes
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(chartX, chartY+chartHeight, chartX+chartWidth, chartY+chartHeight) // X-axis
	pdf.Line(chartX, chartY, chartX, chartY+chartHeight)                         // Y-axis

	// Find min/max values for scaling
	maxAvgTime := 0.0
	for label, totalTime := range rangeAvgTimes {
		count := rangeCounts[label]
		if count > 0 {
			avgTime := (totalTime / float64(count)) / 1000.0 // Convert to seconds
			if avgTime > maxAvgTime {
				maxAvgTime = avgTime
			}
		}
	}

	if maxAvgTime == 0 {
		maxAvgTime = 1 // Avoid division by zero
	}

	// Draw grid lines and labels
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetFont("Arial", "", 8)

	// Y-axis labels (time in seconds)
	for i := 0; i <= 4; i++ {
		yPos := chartY + chartHeight - (float64(i) * chartHeight / 4)
		pdf.Line(chartX, yPos, chartX+chartWidth, yPos)

		timeValue := (float64(i) * maxAvgTime / 4)
		pdf.SetXY(chartX-15, yPos-2)
		pdf.CellFormat(10, 5, fmt.Sprintf("%.2f", timeValue), "", 0, "R", false, 0, "")
	}

	// X-axis labels (token ranges)
	for i, r := range ranges {
		xPos := chartX + (float64(i) * chartWidth / float64(len(ranges)-1))
		pdf.Line(xPos, chartY, xPos, chartY+chartHeight)
		pdf.SetXY(xPos-5, chartY+chartHeight+2)
		pdf.CellFormat(10, 5, r.label, "", 0, "C", false, 0, "")
	}

	// Plot data points as a line chart
	pdf.SetDrawColor(33, 150, 243) // Blue for the line
	pdf.SetLineWidth(0.5)
	var lastX, lastY float64 = -1, -1

	for i, r := range ranges {
		if count, ok := rangeCounts[r.label]; ok && count > 0 {
			avgTime := (rangeAvgTimes[r.label] / float64(count)) / 1000.0 // Convert to seconds

			// Calculate position
			xPos := chartX + (float64(i) * chartWidth / float64(len(ranges)-1))
			yPos := chartY + chartHeight - ((avgTime / maxAvgTime) * chartHeight)

			if lastX != -1 {
				pdf.Line(lastX, lastY, xPos, yPos)
			}
			lastX, lastY = xPos, yPos
		}
	}

	// Add axis labels
	pdf.SetFont("Arial", "", 9)
	pdf.SetXY(chartX+chartWidth/2-20, chartY+chartHeight+10)
	pdf.CellFormat(40, 5, "Token Range", "", 0, "C", false, 0, "")

	pdf.SetXY(chartX-25, chartY+chartHeight/2-5)
	pdf.CellFormat(20, 5, "Avg Time (s)", "", 0, "C", false, 0, "")
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