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

  if (simulationId) {
    return (
      <StatusView 
        simulationId={simulationId} 
        onBack={handleBack}
      />
    );
  }

  return (
    <SimulationForm onSimulationStart={handleSimulationStart} />
  );
};

export default Index;
