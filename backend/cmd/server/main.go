package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rubix-simulator/backend/internal/config"
	"github.com/rubix-simulator/backend/internal/handlers"
	"github.com/rubix-simulator/backend/internal/middleware"
	"github.com/rubix-simulator/backend/internal/services"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

func main() {
	cfg := config.Load()

	nodeManager := services.NewNodeManager(cfg)
	transactionExecutor := services.NewTransactionExecutor(cfg)
	reportGenerator := services.NewReportGenerator(cfg)
	simulationService := services.NewSimulationService(nodeManager, transactionExecutor, reportGenerator)

	handler := handlers.NewHandler(simulationService, reportGenerator)

	// Auto-start token monitoring if nodes already exist
	nodeManager.AutoStartTokenMonitoring()

	// Optional: Start cleanup routine for very old finished simulations
	// (commented out by default - users might want to keep finished reports)
	// go func() {
	// 	ticker := time.NewTicker(1 * time.Hour) // Clean up every hour
	// 	defer ticker.Stop()
	// 	
	// 	for range ticker.C {
	// 		simulationService.CleanupFinishedSimulations()
	// 	}
	// }()

	router := setupRouter(handler)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      c.Handler(middleware.LoggingMiddleware(router)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Starting server on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// NOTE: Nodes are intentionally NOT stopped when server shuts down
	// This allows nodes to continue running independently of the backend server
	// if err := nodeManager.StopAllNodes(); err != nil {
	// 	log.Printf("Error stopping nodes: %v", err)
	// }

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func setupRouter(h *handlers.Handler) *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Node management endpoints
	r.HandleFunc("/nodes/start", h.StartNodes).Methods("POST")
	r.HandleFunc("/nodes/stop", h.StopNodes).Methods("POST")
	r.HandleFunc("/nodes/restart", h.RestartNodes).Methods("POST")
	r.HandleFunc("/nodes/reset", h.ResetNodes).Methods("POST")
	r.HandleFunc("/nodes/check-tokens", h.CheckTokenBalances).Methods("POST")
	r.HandleFunc("/nodes/token-status", h.GetTokenMonitoringStatus).Methods("GET")

	// Simulation endpoints
	r.HandleFunc("/simulate", h.StartSimulation).Methods("POST")
	r.HandleFunc("/report/{id}", h.GetSimulationStatus).Methods("GET")
	r.HandleFunc("/simulations/active", h.GetActiveSimulations).Methods("GET")

	// Report endpoints
	r.HandleFunc("/reports/{id}/download", h.DownloadReport).Methods("GET")
	r.HandleFunc("/reports/list", h.ListReports).Methods("GET")

	return r
}
