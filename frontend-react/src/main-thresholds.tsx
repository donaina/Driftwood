import ReactDOM from 'react-dom/client';
import CustomAlertThresholds from './components/CustomAlertThresholds';

function mountThresholds(container: HTMLElement) {
  ReactDOM.createRoot(container).render(<CustomAlertThresholds />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountThresholds = mountThresholds;
}