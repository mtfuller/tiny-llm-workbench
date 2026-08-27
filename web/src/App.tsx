import { Route, Routes } from 'react-router-dom'
import Layout from './Layout'
import AgentEditor from './pages/AgentEditor'
import Agents from './pages/Agents'
import BenchmarkDetail from './pages/BenchmarkDetail'
import Benchmarks from './pages/Benchmarks'
import DatasetDetail from './pages/DatasetDetail'
import Datasets from './pages/Datasets'
import EnvironmentDetail from './pages/EnvironmentDetail'
import Environments from './pages/Environments'
import EvaluationDetail from './pages/EvaluationDetail'
import Evaluations from './pages/Evaluations'
import Home from './pages/Home'
import Knowledge from './pages/Knowledge'
import KnowledgeDetail from './pages/KnowledgeDetail'
import ModelDetail from './pages/ModelDetail'
import Models from './pages/Models'
import Settings from './pages/Settings'
import Tools from './pages/Tools'
import Training from './pages/Training'
import TrainingRunDetail from './pages/TrainingRunDetail'

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Home />} />
        <Route path="models" element={<Models />} />
        <Route path="models/:name" element={<ModelDetail />} />
        <Route path="datasets" element={<Datasets />} />
        <Route path="datasets/:name" element={<DatasetDetail />} />
        <Route path="training" element={<Training />} />
        <Route path="training/:id" element={<TrainingRunDetail />} />
        <Route path="environments" element={<Environments />} />
        <Route path="environments/:name" element={<EnvironmentDetail />} />
        <Route path="knowledge" element={<Knowledge />} />
        <Route path="knowledge/:name" element={<KnowledgeDetail />} />
        <Route path="tools" element={<Tools />} />
        <Route path="agents" element={<Agents />} />
        <Route path="agents/:name" element={<AgentEditor />} />
        <Route path="evaluations" element={<Evaluations />} />
        <Route path="evaluations/:name" element={<EvaluationDetail />} />
        <Route path="benchmarks" element={<Benchmarks />} />
        <Route path="benchmarks/:name" element={<BenchmarkDetail />} />
        <Route path="settings" element={<Settings />} />
      </Route>
    </Routes>
  )
}

export default App
