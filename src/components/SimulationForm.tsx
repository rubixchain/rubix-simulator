import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AlertCircle, Play, CheckCircle, XCircle } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";

interface SimulationFormProps {
  onSimulationStart: (simulationId: string) => void;
}

export const SimulationForm = ({ onSimulationStart }: SimulationFormProps) => {
  const [additionalNodes, setAdditionalNodes] = useState<string>("");
  const [transactions, setTransactions] = useState<string>("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string>("");
  const [successMessage, setSuccessMessage] = useState<string>("");
  const [backendStatus, setBackendStatus] = useState<"checking" | "connected" | "disconnected">("checking");

  useEffect(() => {
    // Check backend connection on mount
    checkBackendConnection();
    // Check every 5 seconds
    const interval = setInterval(checkBackendConnection, 5000);
    return () => clearInterval(interval);
  }, []);

  const checkBackendConnection = async () => {
    try {
      const response = await fetch("/health", { method: "GET" });
      setBackendStatus(response.ok ? "connected" : "disconnected");
    } catch {
      setBackendStatus("disconnected");
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSuccessMessage("");

    const additionalNodesNum = parseInt(additionalNodes);
    const transactionsNum = parseInt(transactions);

    // Validation
    if (isNaN(additionalNodesNum) || additionalNodesNum < 2 || additionalNodesNum > 20) {
      setError("Number of transaction nodes must be between 2 and 20 (minimum 2 required for sender and receiver)");
      return;
    }

    if (!transactionsNum || transactionsNum < 1 || transactionsNum > 500) {
      setError("Number of transactions must be between 1 and 500");
      return;
    }

    setIsLoading(true);

    try {
      const response = await fetch("/simulate", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          nodes: additionalNodesNum,  // This represents additional nodes beyond the 7 quorum nodes
          transactions: transactionsNum,
        }),
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => null);
        throw new Error(errorData?.message || `Server error: ${response.status}`);
      }

      const data = await response.json();
      if (!data.simulationId) {
        throw new Error("Invalid response from server");
      }
      
      onSimulationStart(data.simulationId);
    } catch (err) {
      if (err instanceof Error) {
        if (err.message.includes("fetch")) {
          setError("Cannot connect to backend server. Please ensure the backend is running on port 8080.");
        } else {
          setError(err.message);
        }
      } else {
        setError("An unexpected error occurred");
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <div className="flex justify-between items-start mb-2">
            <div className="flex-1">
              <CardTitle className="text-2xl font-bold bg-gradient-to-r from-primary to-primary/80 bg-clip-text text-transparent">
                Rubix Network Simulator
              </CardTitle>
            </div>
            <Badge 
              variant={backendStatus === "connected" ? "default" : backendStatus === "checking" ? "secondary" : "destructive"}
              className="ml-2"
            >
              {backendStatus === "connected" && <CheckCircle className="h-3 w-3 mr-1" />}
              {backendStatus === "disconnected" && <XCircle className="h-3 w-3 mr-1" />}
              {backendStatus === "checking" ? "Checking..." : backendStatus === "connected" ? "Backend Connected" : "Backend Offline"}
            </Badge>
          </div>
          <p className="text-muted-foreground">
            Configure and run network transaction simulations
          </p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="nodes">
                Number of Transaction Nodes
                <span className="text-xs text-muted-foreground ml-2">
                  (in addition to 7 quorum nodes)
                </span>
              </Label>
              <Input
                id="nodes"
                type="number"
                min="2"
                max="20"
                value={additionalNodes}
                onChange={(e) => setAdditionalNodes(e.target.value)}
                placeholder="Enter number of transaction nodes (2-20)"
                required
              />
              <p className="text-xs text-muted-foreground mt-1">
                Total nodes: 7 quorum (consensus only) + {additionalNodes || 0} transaction nodes = {7 + (parseInt(additionalNodes) || 0)} nodes
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="transactions">Number of Transactions</Label>
              <Input
                id="transactions"
                type="number"
                min="1"
                max="500"
                value={transactions}
                onChange={(e) => setTransactions(e.target.value)}
                placeholder="Enter number of transactions (1-500)"
                required
              />
            </div>

            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            {successMessage && (
              <Alert className="border-green-500 bg-green-50 dark:bg-green-950/20">
                <CheckCircle className="h-4 w-4 text-green-600" />
                <AlertDescription className="text-green-800 dark:text-green-200">{successMessage}</AlertDescription>
              </Alert>
            )}

            <Button
              type="submit"
              className="w-full"
              disabled={isLoading || backendStatus !== "connected"}
            >
              {isLoading ? (
                <div className="flex items-center space-x-2">
                  <div className="animate-spin rounded-full h-4 w-4 border-2 border-current border-t-transparent" />
                  <span>Starting Simulation...</span>
                </div>
              ) : (
                <div className="flex items-center space-x-2">
                  <Play className="h-4 w-4" />
                  <span>Start Simulation</span>
                </div>
              )}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
};