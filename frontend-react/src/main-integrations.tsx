import { mountView } from './mount';
import IntegrationsLibrary from './components/IntegrationsLibrary';

function mountIntegrations(container: HTMLElement) {
  mountView(container, <IntegrationsLibrary />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountIntegrations = mountIntegrations;
}