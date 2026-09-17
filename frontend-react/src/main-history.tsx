import { mountView } from './mount';
import HistoryView from './components/HistoryView';

/* The single import site for the stylesheet. It lives here only because Vite
   needs CSS reached from a JS entry, and the entry that used to carry it
   (main.tsx) was dead code. With cssCodeSplit:false the whole build emits ONE
   stylesheet, so it does not matter which entry imports it — only that some
   entry does. Removing this line drops assets/driftwood.css from the build
   and the page loses both the shell's styles and every Tailwind utility. */
import './index.css';

function mountHistory(container: HTMLElement) {
  mountView(container, <HistoryView />);
}

// Attach to window for external use
if (typeof window !== 'undefined') {
  (window as any).mountHistory = mountHistory;
}