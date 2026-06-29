import { BrowserRouter, Routes, Route } from 'react-router';
import { Layout } from './components/Layout';
import { CatalogView } from './views/CatalogView';
import { WorkflowsView } from './views/WorkflowsView';
import { OverviewView } from './views/OverviewView';

export function App() {
  return (
    <BrowserRouter basename="/agents">
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<OverviewView />} />
          <Route path="catalog" element={<CatalogView />} />
          <Route path="workflows" element={<WorkflowsView />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
