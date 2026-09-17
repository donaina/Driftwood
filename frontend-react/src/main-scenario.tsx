import { mountView } from './mount';
import ScenarioLibrary from './components/ScenarioLibrary';

function mountScenario(container: HTMLElement) {
  mountView(container, <ScenarioLibrary />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountScenario = mountScenario;
}