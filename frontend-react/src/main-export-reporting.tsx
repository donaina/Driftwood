import ReactDOM from 'react-dom/client';
import ExportReporting from './components/ExportReporting';

function mountExportReporting(container: HTMLElement) {
  ReactDOM.createRoot(container).render(<ExportReporting />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountExportReporting = mountExportReporting;
}