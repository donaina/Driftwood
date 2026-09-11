import ReactDOM from 'react-dom/client';
import ScenarioLibrary from './components/ScenarioLibrary';

function mountScenario(container: HTMLElement) {
  ReactDOM.createRoot(container).render(<ScenarioLibrary />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountScenario = mountScenario;
}