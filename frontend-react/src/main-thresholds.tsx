import { mountView } from './mount';
import CustomAlertThresholds from './components/CustomAlertThresholds';

function mountThresholds(container: HTMLElement) {
  mountView(container, <CustomAlertThresholds />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountThresholds = mountThresholds;
}