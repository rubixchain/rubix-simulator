import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Badge } from "@/components/ui/badge";
import { 
  ArrowLeft, 
  Download, 
  CheckCircle, 
  XCircle, 
  Clock, 
  Activity,
  Network
} from "lucide-react";

interface SimulationReport {
  simulationId: string;
  nodes: Array<{
    id: string;
    port: number;
    isQuorum: boolean;
    status: string;
  }>;
  transactionsCompleted: number;
  totalTransactions: number;
  successCount: number;
  failureCount: number;
  averageLatency: number;
  totalTime: number;
  isFinished: boolean;
  error?: string;
}

interface StatusViewProps {
  simulationId: string;
  onBack: () => void;
}

// Helper function to format duration in human-readable format
const formatDuration = (milliseconds: number): string => {
  if (milliseconds < 1000) {
    return `${milliseconds}ms`;
  }
  
  const seconds = Math.floor(milliseconds / 1000);
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  
  if (minutes > 0) {
    if (remainingSeconds > 0) {
      return `${minutes}m${remainingSeconds}s`;
    }
    return `${minutes}m`;
  }
  return `${seconds}s`;
};

export const StatusView = ({ simulationId, onBack }: StatusViewProps) => {
  const [report, setReport] = useState<SimulationReport | null>(null);
  const [error, setError] = useState<string>("");
  const [isPolling, setIsPolling] = useState(true);

  useEffect(() => {
    const fetchReport = async () => {
      try {
        const response = await fetch(`/report/${simulationId}`);
        if (!response.ok) {
          throw new Error("Failed to fetch simulation report");
        }
        const data = await response.json();
        setReport(data);
        
        if (data.error) {
          setError(data.error);
          setIsPolling(false);
        } else if (data.isFinished) {
          setIsPolling(false);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : "An error occurred");
        setIsPolling(false);
      }
    };

    fetchReport();

    let interval: NodeJS.Timeout;
    if (isPolling) {
      interval = setInterval(fetchReport, 2000);
    }

    return () => {
      if (interval) clearInterval(interval);
    };
  }, [simulationId, isPolling]);

  const handleDownload = async () => {
    try {
      // Download PDF report directly
      const response = await fetch(`/reports/${simulationId}/download`);
      if (!response.ok) {
        throw new Error("Failed to download report");
      }
      
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `simulation-${simulationId}-report.pdf`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to download report");
    }
  };

  if (error) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle className="text-destructive">Error</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-muted-foreground">{error}</p>
            <Button onClick={onBack} variant="outline" className="w-full">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Form
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!report) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="flex items-center justify-center py-8">
            <div className="flex items-center space-x-2">
              <div className="animate-spin rounded-full h-6 w-6 border-2 border-primary border-t-transparent" />
              <span>Loading simulation data...</span>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const progressPercentage = (report.transactionsCompleted / report.totalTransactions) * 100;

  return (
    <div className="min-h-screen bg-background p-4">
      <div className="max-w-4xl mx-auto space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <Button onClick={onBack} variant="outline">
            <ArrowLeft className="h-4 w-4 mr-2" />
            Back to Form
          </Button>
          
          <div className="flex items-center space-x-2">
            <Badge variant={report.isFinished ? "default" : "secondary"}>
              {report.isFinished ? "Completed" : "Running"}
            </Badge>
            {report.isFinished && (
              <Button onClick={handleDownload} size="sm">
                <Download className="h-4 w-4 mr-2" />
                Download Report
              </Button>
            )}
          </div>
        </div>

        {/* Simulation Info */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <Network className="h-5 w-5" />
              <span>Simulation {simulationId}</span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="text-center">
                <div className="text-2xl font-bold text-primary">
                  {report.nodes ? report.nodes.length : 0}
                </div>
                <div className="text-sm text-muted-foreground">Total Nodes</div>
                <div className="text-xs text-muted-foreground mt-1">
                  {report.nodes && (
                    <>
                      {report.nodes.filter(n => n.isQuorum).length} quorum +{' '}
                      {report.nodes.filter(n => !n.isQuorum).length} transaction
                    </>
                  )}
                </div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold">{report.totalTransactions}</div>
                <div className="text-sm text-muted-foreground">Total Transactions</div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold text-success">{report.successCount}</div>
                <div className="text-sm text-muted-foreground">Successful</div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold text-destructive">{report.failureCount}</div>
                <div className="text-sm text-muted-foreground">Failed</div>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Progress */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <Activity className="h-5 w-5" />
              <span>Progress</span>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>Transactions Completed</span>
                <span>{report.transactionsCompleted} / {report.totalTransactions}</span>
              </div>
              <Progress value={progressPercentage} className="h-2" />
              <div className="text-center text-sm text-muted-foreground">
                {progressPercentage.toFixed(1)}% Complete
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-4">
              <div className="flex items-center space-x-3 p-3 bg-muted/50 rounded-lg">
                <CheckCircle className="h-5 w-5 text-success" />
                <div>
                  <div className="font-medium">Success Rate</div>
                  <div className="text-sm text-muted-foreground">
                    {((report.successCount / Math.max(report.transactionsCompleted, 1)) * 100).toFixed(1)}%
                  </div>
                </div>
              </div>

              <div className="flex items-center space-x-3 p-3 bg-muted/50 rounded-lg">
                <Clock className="h-5 w-5 text-primary" />
                <div>
                  <div className="font-medium">Avg Latency</div>
                  <div className="text-sm text-muted-foreground">
                    {formatDuration(report.averageLatency)}
                  </div>
                </div>
              </div>
            </div>

            {report.isFinished && (
              <div className="flex items-center space-x-3 p-3 bg-primary/10 rounded-lg">
                <CheckCircle className="h-5 w-5 text-primary" />
                <div>
                  <div className="font-medium">Total Time</div>
                  <div className="text-sm text-muted-foreground">
                    {formatDuration(report.totalTime)}
                  </div>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        {!report.isFinished && (
          <Card>
            <CardContent className="flex items-center justify-center py-6">
              <div className="flex items-center space-x-2 text-muted-foreground">
                <div className="animate-spin rounded-full h-4 w-4 border-2 border-primary border-t-transparent" />
                <span>Simulation in progress... Updating every 2 seconds</span>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
};