import { lazy } from 'react'
import { Routes, Route, Navigate, useLocation, useParams } from 'react-router-dom'
import App from './App'
import { RequireAdmin, RequireLogin } from './components/RouteGuards'
import { DomainProvider } from './domain/DomainContext'
import { domainPath, defaultDomain } from './domain/paths'
import { useAuth } from './auth/AuthContext'

const HomePage = lazy(() => import('./pages/HomePage'))
const ProblemListPage = lazy(() => import('./pages/ProblemListPage'))
const ProblemSetListPage = lazy(() => import('./pages/ProblemSetListPage'))
const ProblemSetDetailPage = lazy(() => import('./pages/ProblemSetDetailPage'))
const ProblemDetailPage = lazy(() => import('./pages/ProblemDetailPage'))
const SubmissionListPage = lazy(() => import('./pages/SubmissionListPage'))
const SubmissionDetailPage = lazy(() => import('./pages/SubmissionDetailPage'))
const EditorialListPage = lazy(() => import('./pages/EditorialListPage'))
const EditorialDetailPage = lazy(() => import('./pages/EditorialDetailPage'))
const ContestListPage = lazy(() => import('./pages/ContestListPage'))
const ContestDetailPage = lazy(() => import('./pages/ContestDetailPage'))
const JuryConsolePage = lazy(() => import('./pages/contest/JuryConsolePage'))
const ProfilePage = lazy(() => import('./pages/ProfilePage'))
const LoginPage = lazy(() => import('./pages/LoginPage'))
const AdminConsolePage = lazy(() => import('./pages/admin/AdminConsolePage'))
const AdminProblemPage = lazy(() => import('./pages/admin/AdminProblemPage'))
const ProblemWorkspacePage = lazy(() => import('./pages/admin/ProblemWorkspacePage'))
const AdminContestPage = lazy(() => import('./pages/admin/AdminContestPage'))
const NotFoundPage = lazy(() => import('./pages/NotFoundPage'))
const DomainDirectoryPage = lazy(() => import('./pages/domain/DomainDirectoryPage'))
const DomainSettingsLayout = lazy(() => import('./pages/domain/DomainSettingsLayout'))
const DomainSettingsPage = lazy(() => import('./pages/domain/DomainSettingsPage'))
const DomainMembersPage = lazy(() => import('./pages/domain/DomainMembersPage'))
const DomainRolesPage = lazy(() => import('./pages/domain/DomainRolesPage'))
const GroupListPage = lazy(() => import('./pages/domain/GroupListPage'))
const GroupDetailPage = lazy(() => import('./pages/domain/GroupDetailPage'))
const TagListPage = lazy(() => import('./pages/domain/TagListPage'))
const TagDetailPage = lazy(() => import('./pages/domain/TagDetailPage'))
const AnnouncementListPage = lazy(() => import('./pages/domain/AnnouncementListPage'))
const AnnouncementDetailPage = lazy(() => import('./pages/domain/AnnouncementDetailPage'))

export default function RootRoutes() {
  return (
    <Routes>
      <Route path="/d/:domain" element={<DomainLayout />}>
        <Route index element={<HomePage />} />
        <Route path="problems" element={<ProblemListPage />} />
        <Route path="problems/:id" element={<ProblemDetailPage />} />
        <Route path="problem-sets" element={<ProblemSetListPage />} />
        <Route path="problem-sets/:id" element={<ProblemSetDetailPage />} />
        <Route path="editorials" element={<EditorialListPage />} />
        <Route path="editorials/:id" element={<EditorialDetailPage />} />
        <Route path="announcements" element={<AnnouncementListPage />} />
        <Route path="announcements/:id" element={<AnnouncementDetailRoute />} />
        <Route path="contests" element={<ContestListPage />} />
        <Route path="contests/:id" element={<ContestDetailRoute />} />
        <Route path="users/:username" element={<ProfilePage />} />
        <Route element={<RequireLogin />}>
          <Route path="contests/:contestId/problems/:id" element={<ProblemDetailPage />} />
          <Route path="contests/:id/jury" element={<JuryConsolePage />} />
          <Route path="submissions" element={<SubmissionListPage />} />
          <Route path="submissions/:id" element={<SubmissionDetailPage />} />
          <Route path="authoring" element={<AdminProblemPage />} />
          <Route path="authoring/:id" element={<ProblemWorkspacePage />} />
          <Route path="manage/contests" element={<AdminContestPage />} />
          <Route path="settings" element={<DomainSettingsLayout />}>
            <Route index element={<DomainSettingsPage />} />
            <Route path="members" element={<DomainMembersPage />} />
            <Route path="roles" element={<DomainRolesPage />} />
            <Route path="tags" element={<TagListPage />} />
            <Route path="tags/:tag" element={<TagDetailRoute />} />
            <Route path="announcements" element={<AnnouncementListPage manage />} />
            <Route path="announcements/:id" element={<AnnouncementDetailRoute manage />} />
          </Route>
          <Route path="groups" element={<GroupListPage />} />
          <Route path="groups/:group" element={<GroupDetailRoute />} />
        </Route>
        <Route path="*" element={<NotFoundPage />} />
      </Route>
      <Route element={<App />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/domains" element={<DomainDirectoryRoute />} />
        <Route element={<RequireAdmin />}>
          <Route path="/admin" element={<AdminConsolePage />} />
        </Route>
        <Route path="*" element={<LegacyRoute />} />
      </Route>
    </Routes>
  )
}

function GroupDetailRoute() {
  const { group } = useParams()
  return <GroupDetailPage key={group} />
}

function ContestDetailRoute() {
  const { id } = useParams()
  return <ContestDetailPage key={id} />
}

function TagDetailRoute() {
  const { tag } = useParams()
  return <TagDetailPage key={tag} />
}
function AnnouncementDetailRoute({ manage = false }: { manage?: boolean }) {
  const { id } = useParams()
  return <AnnouncementDetailPage key={`${manage}:${id}`} manage={manage} />
}

function DomainDirectoryRoute() {
  const { user, ready } = useAuth()
  return <DomainDirectoryPage key={`${user?.id}:${ready}`} />
}

function DomainLayout() {
  const { domain } = useParams()
  const { user } = useAuth()
  return (
    <DomainProvider
      key={`${domain}:${user?.id}:${user?.role}`}
      frame={(content) => <App>{content}</App>}
    >
      <App />
    </DomainProvider>
  )
}

function LegacyRoute() {
  const location = useLocation()
  const target = domainPath(defaultDomain, location.pathname)
  return target !== location.pathname ? (
    <Navigate replace to={`${target}${location.search}${location.hash}`} />
  ) : (
    <NotFoundPage />
  )
}
