import ReactDOM from 'react-dom/client';
import AgencyMode from './components/AgencyMode';

function mountAgency(container: HTMLElement) {
  ReactDOM.createRoot(container).render(<AgencyMode />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountAgency = mountAgency;
}