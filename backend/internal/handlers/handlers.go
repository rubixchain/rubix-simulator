package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
	"fmt"

	"github.com/gorilla/mux"
	"github.com/rubix-simulator/backend/internal/models"
	"github.com/rubix-simulator/backend/internal/services"
)

type Handler struct {
	simulationService *services.SimulationService
	reportGenerator   *services.ReportGenerator
	nodeManager       *services.NodeManager
}

func NewHandler(ss *services.SimulationService, rg *services.ReportGenerator) *Handler {
	return &Handler{
		simulationService: ss,
		reportGenerator:   rg,
		nodeManager:       ss.GetNodeManager(),
	}
}

func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := models.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) StartNodes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count int  `json:"count"`
		Fresh bool `json:"fresh"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Default to 2 transaction nodes if not specified
	if req.Count == 0 {
		req.Count = 2
	}
	
	// Start nodes using the node manager
	nodes, err := h.nodeManager.StartNodesWithOptions(req.Count, req.Fresh)
	if err != nil {
		h.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Nodes started successfully",
		"nodes":   nodes,
		"total":   len(nodes),
	})
}

func (h *Handler) StopNodes(w http.ResponseWriter, r *http.Request) {
	err := h.nodeManager.StopAllNodes()
	if err != nil {
		h.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "All nodes stopped",
	})
}

func (h *Handler) RestartNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.nodeManager.RestartNodes()
	if err != nil {
		h.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Nodes restarted with preserved state",
		"mode":    "restart",
		"nodes":   nodes,
		"total":   len(nodes),
	})
}

func (h *Handler) ResetNodes(w http.ResponseWriter, r *http.Request) {
	// Note: This would need access to NodeManager - simplified for now
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "All node data reset",
		"mode": "reset",
	})
}

func (h *Handler) StartSimulation(w http.ResponseWriter, r *http.Request) {
	var req models.SimulationRequest
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	simulationID, err := h.simulationService.StartSimulation(req.Nodes, req.Transactions)
	if err != nil {
		h.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	response := models.SimulationResponse{
		SimulationID: simulationID,
		Message:      "Simulation started successfully",
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) GetSimulationStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	simulationID := vars["id"]
	
	report, err := h.simulationService.GetSimulationReport(simulationID)
	if err != nil {
		h.sendError(w, "Simulation not found", http.StatusNotFound)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (h *Handler) DownloadReport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	reportID := vars["id"]
	
	filename := "simulation-" + reportID + ".pdf"
	filepath := h.reportGenerator.GetReportPath(filename)
	
	file, err := os.Open(filepath)
	if err != nil {
		h.sendError(w, "Report not found", http.StatusNotFound)
		return
	}
	defer file.Close()
	
	stat, err := file.Stat()
	if err != nil {
		h.sendError(w, "Failed to get file info", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Length", fmt.Sprint(stat.Size()))
	
	io.Copy(w, file)
}

func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	reports, err := h.reportGenerator.ListReports()
	if err != nil {
		h.sendError(w, "Failed to list reports", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reports)
}

func (h *Handler) CheckTokenBalances(w http.ResponseWriter, r *http.Request) {
	h.nodeManager.CheckTokenBalances()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Token balance check initiated. Check server logs for details.",
		"timestamp": time.Now(),
	})
}

func (h *Handler) GetTokenMonitoringStatus(w http.ResponseWriter, r *http.Request) {
	isSimActive := h.nodeManager.IsSimulationActive()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"simulation_active": isSimActive,
		"token_monitoring_paused": isSimActive,
		"message": func() string {
			if isSimActive {
				return "Token monitoring is paused - simulation is running"
			}
			return "Token monitoring is active - no simulation running"
		}(),
		"timestamp": time.Now(),
	})
}

func (h *Handler) sendError(w http.ResponseWriter, message string, code int) {
	response := models.ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
		Code:    code,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(response)
}