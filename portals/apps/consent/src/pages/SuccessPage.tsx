/**
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Lanka Software Foundation
 */

import { CheckCircle } from 'lucide-react';
import React from 'react';
import { useAuth } from "react-oidc-context";
import UserHeader from '../components/UserHeader';

const SuccessPage: React.FC = () => {
  const auth = useAuth();
  const user = auth.user?.profile;
  const userName = user?.given_name || user?.name || user?.email || user?.preferred_username || user?.sub || 'User';

  return (
    <div className="min-h-screen bg-gradient-to-br from-green-50 to-emerald-100 flex items-center justify-center p-4 relative">
      {auth.isAuthenticated && <UserHeader userName={userName} onSignIn={() => auth.signinRedirect()} onSignOut={() => auth.signoutRedirect()} />}
      <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
        <CheckCircle className="h-12 w-12 text-green-500 mx-auto mb-4" />
        <h1 className="text-xl font-bold text-gray-800 mb-2">Success!</h1>
        <p className="text-gray-600 mb-4">
          Your consent has been processed successfully.
        </p>
        <p className="text-sm text-gray-500">
          Redirecting you back to the application...
        </p>
      </div>
    </div>
  );
};

export default SuccessPage;