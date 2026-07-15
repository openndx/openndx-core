import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { useAuthContext } from "@asgardeo/auth-react";
import { Navbar } from "./components/Navbar";
import { Schemas } from './pages/Schemas';
import { Logs } from "./pages/Logs";
import { Applications } from "./pages/Applications";
import { useEffect, useState } from "react";
import { Shield } from 'lucide-react';
import { Home } from './pages/Home';
import { Members } from './pages/Members';

function App() {
  const { state, signIn, signOut, getBasicUserInfo } = useAuthContext();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [isSigningIn, setIsSigningIn] = useState(false);
  const [isSigningOut, setIsSigningOut] = useState(false);

  useEffect(() => {
    const fetchUserInfo = async () => {
      // If authentication is still being determined, keep loading
      if (state.isLoading) {
        return;
      }

      // If user is signing out, don't try to fetch user info
      if (isSigningOut) {
        return;
      }

      if (!state.isAuthenticated) {
        setLoading(false);
        return;
      }

      try {
        // Fetch fresh data from API
        const userBasicInfo = await getBasicUserInfo();
        console.log('Fetching entity info from user attributes:', userBasicInfo);

        if (!userBasicInfo || !userBasicInfo.roles) {
          setError('Failed to fetch valid entity info from user attributes');
        }
        else if (!userBasicInfo.roles.includes(window.configs.VITE_IDP_ADMIN_ROLE)) {
          setError('Failed to fetch valid entity info from DB or user is not an admin');
        }
      } catch (error) {
        console.error('Failed to fetch entity info:', error);
        setError('An error occurred while fetching entity info');
      } finally {
        setLoading(false);
        setIsSigningIn(false);
        setIsSigningOut(false);
      }
    };
    fetchUserInfo();
  }, [state.isAuthenticated, state.isLoading, isSigningIn, isSigningOut, getBasicUserInfo]);

  const handleSignIn = () => {
    setIsSigningIn(true);
    setError(null);
    signIn();
  };

  const handleSignOut = () => {
    setIsSigningOut(true);
    setError(null);
    signOut();
  };

  // Show loading while fetching user data or during authentication
  if (loading || state.isLoading || isSigningIn || isSigningOut) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center relative">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">
            {isSigningIn ? 'Signing in...' : isSigningOut ? 'Signing out...' : 'Loading...'}
          </p>
        </div>
      </div>
    );
  }

  // Show login screen if not authenticated
  if (!state.isAuthenticated) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4 relative">
        <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
          <Shield className="h-12 w-12 text-blue-500 mx-auto mb-4" />
          <h1 className="text-2xl font-bold text-gray-800 mb-4">Admin Portal</h1>
          <p className="text-gray-600 mb-4">
            Sign in to access the OpenDIF Admin Portal.
          </p>
          <button
            onClick={handleSignIn}
            className="bg-blue-500 hover:bg-blue-600 text-white px-6 py-3 rounded-lg font-medium transition-colors"
          >
            Sign In to Continue
          </button>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4 relative">
        <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
          <h1 className="text-2xl font-bold text-red-600 mb-4">Error</h1>
          <p className="text-gray-600 mb-4">{error}</p>
          <button
            onClick={handleSignOut}
            className="bg-red-500 hover:bg-red-600 text-white px-6 py-3 rounded-lg font-medium transition-colors"
          >
            Sign Out
          </button>
        </div>
      </div>
    );
  }

  return (
    <Router>
      <div className="App h-screen flex">
        <Navbar
          onSignOut={handleSignOut}
        />
        <main className="flex-1 overflow-auto pt-16">
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/members" element={<Members />} />
            <Route path="/schemas" element={<Schemas />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="/applications" element={<Applications />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
    </Router>
  );
}

export default App;