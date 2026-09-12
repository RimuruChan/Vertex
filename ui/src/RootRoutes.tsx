import { lazy } from 'react'
import { Routes, Route, Navigate, useLocation, useParams } from 'react-router-dom'
import App from './App'
import { RequireAdmin, RequireLogin } from './components/RouteGuards'
import { DomainProvider, useDomain } from './domain/DomainContext'
import { domainPath, defaultDomain } from './domain/paths'
import { useAuth } from './auth/AuthContext'
import { ContestProvider, useContestSpace } from './components/contest/ContestContext'
import { contestSections } from './lib/contest-routes'

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
const WorkspaceLayout = lazy(() => import('./pages/workspace/WorkspaceLayout'))
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
        <Route path="contests/:id" element={<ContestLandingRoute />} />
        <Route path="contests/:id/:section" element={<ContestSectionRoute />} />
        <Route path="users/:username" element={<ProfilePage />} />
        <Route element={<RequireLogin />}>
          <Route path="contests/:contestId/submissions" element={<SubmissionListPage />} />
          <Route path="contests/:contestId/submissions/:id" element={<SubmissionDetailPage />} />
          <Route path="contests/:contestId/problems/:id" element={<ProblemDetailPage />} />
          <Route path="contests/:id/rejudge" element={<JuryConsolePage activeTab="rejudge" />} />
          <Route path="submissions" element={<SubmissionListPage />} />
          <Route path="submissions/:id" element={<SubmissionDetailPage />} />
          <Route path="workspace" element={<WorkspaceLayout />}>
            <Route index element={<DomainRedirect to="/workspace/problems" />} />
            <Route path="problems" element={<AdminProblemPage />} />
            <Route path="contests" element={<AdminContestPage />} />
          </Route>
          <Route path="authoring" element={<DomainRedirect to="/workspace/problems" />} />
          <Route path="authoring/:id" element={<ProblemWorkspacePage />} />
          <Route path="manage/contests" element={<DomainRedirect to="/workspace/contests" />} />
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

function DomainRedirect({ to }: { to: string }) {
  const { slug } = useDomain()
  const { search, hash } = useLocation()
  return <Navigate replace to={`${domainPath(slug, to)}${search}${hash}`} />
}

function ContestLandingRoute() {
  const { id } = useParams()
  const { slug } = useDomain()
  const space = useContestSpace()
  return (
    <Navigate replace to={domainPath(slug, space?.workspaceHref ?? `/contests/${id}/problems`)} />
  )
}

function ContestSectionRoute() {
  const { id, section = '' } = useParams()
  const space = useContestSpace()
  const activeTab = Object.prototype.hasOwnProperty.call(contestSections, section)
    ? contestSections[section]
    : undefined
  if (!activeTab) return <NotFoundPage />
  if (
    space?.details?.contest.permissions.viewJury &&
    ['standings', 'clarifications'].includes(section)
  ) {
    return <JuryConsolePage activeTab={section === 'standings' ? 'board' : 'clarifications'} />
  }
  return <ContestDetailPage key={id} activeTab={activeTab} />
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
      <ContestProvider>
        <App />
      </ContestProvider>
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
