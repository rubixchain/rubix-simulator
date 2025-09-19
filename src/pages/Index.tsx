import { useState, useEffect } from "react";
import { SimulationForm } from "@/components/SimulationForm";
import { StatusView } from "@/components/StatusView";

const Index = () => {
  const [simulationId, setSimulationId] = useState<string>("");
  const [isCheckingActiveSimulations, setIsCheckingActiveSimulations] = useState(true);

  // Check for active simulations on page load
  useEffect(() => {
    const checkActiveSimulations = async () => {
      try {
        const response = await fetch("/simulations/active");
        if (response.ok) {
          const data = await response.json();
          if (data.active_simulations && data.active_simulations.length > 0) {
            // If there are active simulations, show the most recent one
            const mostRecent = data.active_simulations[0];
            setSimulationId(mostRecent.simulationId);
            console.log("Recovered active simulation:", mostRecent.simulationId);
          }
        }
      } catch (error) {
        console.error("Failed to check for active simulations:", error);
      } finally {
        setIsCheckingActiveSimulations(false);
      }
    };

    // Check localStorage first for immediate recovery
    const savedSimulationId = localStorage.getItem("currentSimulationId");
    if (savedSimulationId) {
      setSimulationId(savedSimulationId);
    }

    checkActiveSimulations();
  }, []);

  const handleSimulationStart = (id: string) => {
    setSimulationId(id);
    localStorage.setItem("currentSimulationId", id);
  };

  const handleBack = () => {
    setSimulationId("");
    localStorage.removeItem("currentSimulationId");
  };

  return (
    <div className="min-h-screen bg-background text-foreground flex flex-col">
      <nav className="bg-secondary">
        <div className="container mx-auto px-4">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center">
              <img src="/rubixNewLogoWhiteText.png" alt="Rubix Logo" className="h-12 w-auto object-contain" />
            </div>
            
          </div>
        </div>
      </nav>
      <main className="flex-grow flex items-center justify-center">
        <div className="container mx-auto p-4">
          {isCheckingActiveSimulations ? (
            <div className="flex items-center justify-center py-8">
              <div className="flex items-center space-x-2">
                <div className="animate-spin rounded-full h-6 w-6 border-2 border-primary border-t-transparent" />
                <span>Checking for active simulations...</span>
              </div>
            </div>
          ) : simulationId ? (
            <StatusView simulationId={simulationId} onBack={handleBack} />
          ) : (
            <SimulationForm onSimulationStart={handleSimulationStart} />
          )}
        </div>
      </main>
    </div>
  );
};

export default Index;
