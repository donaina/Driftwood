import ReactDOM from 'react-dom/client';
import HistoryView from './components/HistoryView';

function mountHistory(container: HTMLElement) {
  ReactDOM.createRoot(container).render(<HistoryView />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountHistory = mountHistory;
}