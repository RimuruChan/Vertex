import { Routes, Route } from 'react-router-dom'
import App from './App'
import HomePage from './pages/HomePage'
import ProblemListPage from './pages/ProblemListPage'
import ProblemDetailPage from './pages/ProblemDetailPage'
import SubmissionListPage from './pages/SubmissionListPage'
import SubmissionDetailPage from './pages/SubmissionDetailPage'
import ContestListPage from './pages/ContestListPage'
import LoginPage from './pages/LoginPage'
import AdminProblemPage from './pages/admin/AdminProblemPage'

export default function RootRoutes() {
  return (
    <Routes>
      <Route element={<App />}>
        <Route path="/" element={<HomePage />} />
        <Route path="/problems" element={<ProblemListPage />} />
        <Route path="/problems/:id" element={<ProblemDetailPage />} />
        <Route path="/submissions" element={<SubmissionListPage />} />
        <Route path="/submissions/:id" element={<SubmissionDetailPage />} />
        <Route path="/contests" element={<ContestListPage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/admin/problems" element={<AdminProblemPage />} />
      </Route>
    </Routes>
  )
}
