import { Route, Routes } from 'react-router-dom'
import Layout from './Layout'
import AgentEditor from './pages/AgentEditor'
import Agents from './pages/Agents'
import DatasetDetail from './pages/DatasetDetail'
import Datasets from './pages/Datasets'
import Environments from './pages/Environments'
import EvaluationDetail from './pages/EvaluationDetail'
import Evaluations from './pages/Evaluations'
import Home from './pages/Home'
import Models from './pages/Models'
import Settings from './pages/Settings'
import Training from './pages/Training'

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Home />} />
        <Route path="models" element={<Models />} />
        <Route path="datasets" element={<Datasets />} />
        <Route path="datasets/:name" element={<DatasetDetail />} />
        <Route path="training" element={<Training />} />
        <Route path="environments" element={<Environments />} />
        <Route path="agents" element={<Agents />} />
        <Route path="agents/:name" element={<AgentEditor />} />
        <Route path="evaluations" element={<Evaluations />} />
        <Route path="evaluations/:name" element={<EvaluationDetail />} />
        <Route path="settings" element={<Settings />} />
      </Route>
    </Routes>
  )
}

export default App
