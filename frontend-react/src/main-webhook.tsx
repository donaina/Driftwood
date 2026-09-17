import { mountView } from './mount';
import WebhookIntegrations from './components/WebhookIntegrations';

function mountWebhook(container: HTMLElement) {
  mountView(container, <WebhookIntegrations />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountWebhook = mountWebhook;
}