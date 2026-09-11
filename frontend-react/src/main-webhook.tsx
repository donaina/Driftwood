import ReactDOM from 'react-dom/client';
import WebhookIntegrations from './components/WebhookIntegrations';

function mountWebhook(container: HTMLElement) {
  ReactDOM.createRoot(container).render(<WebhookIntegrations />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountWebhook = mountWebhook;
}