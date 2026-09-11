import ReactDOM from 'react-dom/client';
import IntegrationsLibrary from './components/IntegrationsLibrary';

function mountIntegrations(container: HTMLElement) {
  ReactDOM.createRoot(container).render(<IntegrationsLibrary />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountIntegrations = mountIntegrations;
}