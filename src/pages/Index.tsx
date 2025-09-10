import { useState } from "react";
import { SimulationForm } from "@/components/SimulationForm";
import { StatusView } from "@/components/StatusView";

const Index = () => {
  const [simulationId, setSimulationId] = useState<string>("");

  const handleSimulationStart = (id: string) => {
    setSimulationId(id);
  };

  const handleBack = () => {
    setSimulationId("");
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
          {simulationId ? (
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
