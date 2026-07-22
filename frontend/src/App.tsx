import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ThemeProvider, BaseStyles, Spinner, Box, Flash } from '@primer/react';
import { AuthProvider, useAuth } from './hooks/useAuth';
import { useWebSocket } from './hooks/useWebSocket';
import { LoginPage } from './pages/LoginPage';
import { ChangePasswordPage } from './pages/ChangePasswordPage';
import { DashboardPage } from './pages/DashboardPage';
import { ScoringPage } from './pages/ScoringPage';
import { AdminSessionPage } from './pages/AdminSessionPage';
import { AdminScorerPage } from './pages/AdminScorerPage';
import { PatrolOverviewPage } from './pages/PatrolOverviewPage';
import { PatrolDetailPage } from './pages/PatrolDetailPage';
import type { ReactNode } from 'react';

const isCampChiefAccount = (user: { id: string; username: string } | null) => (
  user?.username === 'campchief' || user?.id === 'usr-campchief'
);

const ProtectedRoute = ({ children }: { children: ReactNode }) => {
  const { user, loading, passwordChangeRequired } = useAuth();

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  if (passwordChangeRequired) {
    return <Navigate to="/change-password" replace />;
  }

  return <>{children}</>;
};

const WebSocketProvider = ({ children }: { children: ReactNode }) => {
  const { user } = useAuth();
  const { status } = useWebSocket(!!user);
  const showConnectionBanner = !!user && (status === 'reconnecting' || status === 'disconnected');

  const bannerText = status === 'reconnecting'
    ? 'Live updates connection lost. Reconnecting...'
    : 'Live updates are offline. Try refreshing if this persists.';

  // Key on user ID so the entire tree remounts on user change,
  // clearing all component-level state (scores, sessions, etc.)
  return (
    <>
      {showConnectionBanner && (
        <Box position="sticky" top={0} zIndex={1000} px={3} pt={2}>
          <Flash variant={status === 'reconnecting' ? 'warning' : 'danger'}>{bannerText}</Flash>
        </Box>
      )}
      <div key={user?.id ?? 'anon'}>{children}</div>
    </>
  );
};

const AdminOnlyRoute = ({ children }: { children: ReactNode }) => {
  const { user } = useAuth();
  if (!user?.is_admin || isCampChiefAccount(user)) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
};

const CampChiefOnlyRoute = ({ children }: { children: ReactNode }) => {
  const { user } = useAuth();
  if (!isCampChiefAccount(user)) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
};

const AppRoutes = () => (
  <WebSocketProvider>
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/change-password" element={<ChangePasswordPage />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <DashboardPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/sessions/:sessionId"
        element={
          <ProtectedRoute>
            <ScoringPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/patrols"
        element={
          <ProtectedRoute>
            <PatrolOverviewPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/patrols/:patrolId"
        element={
          <ProtectedRoute>
            <PatrolDetailPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/sessions/:sessionId"
        element={
          <ProtectedRoute>
            <AdminOnlyRoute>
              <AdminSessionPage />
            </AdminOnlyRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/admin/sessions/:sessionId/scorer/:userId"
        element={
          <ProtectedRoute>
            <AdminOnlyRoute>
              <AdminScorerPage />
            </AdminOnlyRoute>
          </ProtectedRoute>
        }
      />
      <Route
        path="/campchief/sessions/:sessionId"
        element={
          <ProtectedRoute>
            <CampChiefOnlyRoute>
              <AdminSessionPage />
            </CampChiefOnlyRoute>
          </ProtectedRoute>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  </WebSocketProvider>
);

export const App = () => (
  <ThemeProvider colorMode="auto">
    <BaseStyles>
      <BrowserRouter>
        <AuthProvider>
          <AppRoutes />
        </AuthProvider>
      </BrowserRouter>
    </BaseStyles>
  </ThemeProvider>
);
