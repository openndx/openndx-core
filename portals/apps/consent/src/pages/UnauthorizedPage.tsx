/**
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Lanka Software Foundation
 */

import { X } from 'lucide-react';
import React from 'react';
import { useAuth } from "react-oidc-context";
import UserHeader from '../components/UserHeader';
import { useConsent } from '../contexts/ConsentContext';

const UnauthorizedPage: React.FC = () => {
  const { error } = useConsent();

  const auth = useAuth();
  const user = auth.user?.profile;
  const userName = user?.given_name || user?.name || user?.email || user?.preferred_username || user?.sub || null;
  const userEmail = user?.email || 'N/A';

  return (
    <div className="min-h-screen bg-linear-to-br from-orange-50 to-red-100 flex items-center justify-center p-4 relative">
      {auth.isAuthenticated && <UserHeader userName={userName} onSignIn={() => auth.signinRedirect()} onSignOut={() => auth.signoutRedirect()} />}
      <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
        <X className="h-12 w-12 text-orange-500 mx-auto mb-4" />
        <h1 className="text-xl font-bold text-gray-800 mb-2">Unauthorized Access</h1>
        <p className="text-gray-600 mb-4">{error}</p>
        <p className="text-sm text-gray-500 mb-4">
          Your email: <span className="font-mono text-blue-600">{userEmail}</span>
        </p>
        <button
          onClick={() => auth.signoutRedirect()}
          className="bg-orange-600 text-white px-6 py-2 rounded-lg hover:bg-orange-700 transition-colors"
        >
          Sign Out
        </button>
      </div>
    </div>
  );
};

export default UnauthorizedPage;