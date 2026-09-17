/* Mounts a React view into a container — once.

   Every `mount*` entry used to call `ReactDOM.createRoot(container).render(...)`
   directly, which is only correct the first time. `createRoot` on a container
   that already has a root does not re-render it: React logs "You are calling
   React.createRoot() on a container that has already been passed to
   createRoot() before", builds a second root over the same DOM, and leaves the
   first one mounted and unreachable. Show a destination twice and there are two
   roots competing for the same element.

   That stayed invisible only because nothing routed to these views, so each was
   mounted at most once. It became reachable the moment the nav entries came
   back, which is when navigating away from Version History and returning to it
   would have hit it every time.

   One root per container, kept in a WeakMap so a detached container is
   collectable, and `render` called on it thereafter — which is React's own
   documented way to update an existing root.

   The map is per-bundle rather than global, and that is sufficient: each
   `*-react-root` has exactly one entry that ever mounts into it (scenario.js
   owns scenario-react-root, history.js owns history-react-root, and so on), so
   each container only ever meets the copy of this module that it needs. */
import ReactDOM from 'react-dom/client';
import type { ReactNode } from 'react';

const roots = new WeakMap<HTMLElement, ReactDOM.Root>();

export function mountView(container: HTMLElement, view: ReactNode): void {
  let root = roots.get(container);
  if (!root) {
    root = ReactDOM.createRoot(container);
    roots.set(container, root);
  }
  root.render(view);
}
