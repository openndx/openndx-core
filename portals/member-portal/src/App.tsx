import { BrowserRouter as Router, Navigate, Route, Routes } from 'react-router-dom';
import { useAuthContext } from "@asgardeo/auth-react";
import { SideNavbar } from "./components/Navbar";
import { SchemasPage } from './pages/Schemas';
import { SchemaRegistrationPage } from "./pages/SchemaRegistrationPage";
import { Logs } from "./pages/Logs";
import { ApplicationsPage as Applications } from "./pages/Applications";
import { useEffect, useState } from "react";
import { ApplicationRegistration } from './pages/ApplicationRegistration';
import { Shield } from 'lucide-react';
import MemberInfo from './pages/MemberInfo';

interface MemberProps {
  memberId: string;
  name: string;
  email: string;
  phoneNumber: string;
  createdAt: string;
  updatedAt: string;
  idpUserId: string;
}

function App() {
  const [memberData, setMemberData] = useState<MemberProps | null>(null);
  const { state, signIn, signOut, getBasicUserInfo } = useAuthContext();
  const [loading, setLoading] = useState(false);

  // Save member state to localStorage to persist through auth redirects
  const saveMemberStateToStorage = (memberInfo: MemberProps) => {
    localStorage.setItem('member_data', JSON.stringify(memberInfo));
  };

  // Get member state from localStorage
  const getMemberStateFromStorage = (): { memberData: MemberProps | null } => {
    try {
      const memberDataStr = localStorage.getItem('member_data');

      return {
        memberData: memberDataStr ? JSON.parse(memberDataStr) : null,
      };
    } catch (error) {
      console.error('Failed to parse stored member data:', error);
      return { memberData: null };
    }
  };

  // Clear member state from localStorage
  const clearMemberStateFromStorage = () => {
    localStorage.removeItem('member_data');
  };

  const fetchMemberInfoFromDB = async (idpUserId: string) => {
    try {
      const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
      // fetch member info from API
      const url = new URL(`${baseUrl}/members`);
      url.searchParams.append('idpUserId', idpUserId);

      const response = await fetch(url.toString());
      if (!response.ok) {
        throw new Error('Failed to fetch member info');
      }
      const data = await response.json();
      if (data.count !== 1) {
        console.error('Member info not found or multiple entries returned');
        return null;
      }
      return data.items[0];
    } catch (error) {
      console.error('Error fetching member info:', error);
      return null;
    }
  };

  useEffect(() => {
    const fetchMemberInfo = async () => {
      if (!state.isAuthenticated) {
        return;
      }

      setLoading(true);

      try {
        // First check localStorage for existing data
        const storedState = getMemberStateFromStorage();
        if (storedState.memberData) {
          console.log('Loading member data from storage');
          setMemberData(storedState.memberData);
          return;
        }

        // Fetch fresh data from API
        const userBasicInfo = await getBasicUserInfo();
        console.log('Fetching member info from user attributes:', userBasicInfo);

        const idpUserId = userBasicInfo.sub;
        if (!idpUserId) {
          throw new Error('User does not have an idpUserId attribute');
        }

        const fetchedMemberInfoFromDB = await fetchMemberInfoFromDB(idpUserId);
        if (fetchedMemberInfoFromDB) {
          const memberInfo: MemberProps = {
            memberId: fetchedMemberInfoFromDB.memberId || '',
            name: fetchedMemberInfoFromDB.name || '',
            email: fetchedMemberInfoFromDB.email || '',
            phoneNumber: fetchedMemberInfoFromDB.phoneNumber || '',
            createdAt: fetchedMemberInfoFromDB.createdAt || '',
            updatedAt: fetchedMemberInfoFromDB.updatedAt || '',
            idpUserId: fetchedMemberInfoFromDB.idpUserId || '',
          };
          console.log('Parsed member info from DB:', memberInfo);

          setMemberData(memberInfo);

          // Save to localStorage for auth redirect recovery
          saveMemberStateToStorage(memberInfo);
        } else {
          // Fallback to empty member data if fetch fails
          const emptyMemberData: MemberProps = {
            memberId: '',
            name: '',
            email: '',
            phoneNumber: '',
            createdAt: '',
            updatedAt: '',
            idpUserId: '',
          };
          setMemberData(emptyMemberData);
        }
      } catch (error) {
        console.error('Failed to fetch member info:', error);
        // Fallback to empty member data if fetch fails
        const emptyMemberData: MemberProps = {
          memberId: '',
          name: '',
          email: '',
          phoneNumber: '',
          createdAt: '',
          updatedAt: '',
          idpUserId: '',
        };
        setMemberData(emptyMemberData);
      } finally {
        setLoading(false);
      }
    };

    fetchMemberInfo();
  }, [state.isAuthenticated, getBasicUserInfo]);

  const handleSignIn = () => {
    signIn();
  };

  const handleSignOut = () => {
    clearMemberStateFromStorage();
    setMemberData(null);
    signOut();
  };

  // Show login screen if not authenticated
  if (!state.isAuthenticated) {
    return (
      <div
        className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4 relative">
        <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
          <Shield className="h-12 w-12 text-blue-500 mx-auto mb-4" />
          <h1 className="text-2xl font-bold text-gray-800 mb-4">Member Portal</h1>
          <p className="text-gray-600 mb-4">
            Sign in to access the OpenNDX Member Portal.
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

  // Show loading while fetching member data
  if (loading || !memberData) {
    return (
      <div
        className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center relative">
        <div className="text-center">
          <div
            className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading member information...</p>
        </div>
      </div>
    );
  }

  return (
    <Router>
      <div className="App">
        <div className="App h-screen flex">
          <SideNavbar onSignOut={handleSignOut} />
          <main className="flex-1 overflow-auto pt-16">
            <Routes>
              <Route path="/" element={<MemberInfo
                name={memberData.name}
                email={memberData.email}
                phoneNumber={memberData.phoneNumber}
                createdAt={memberData.createdAt}
                updatedAt={memberData.updatedAt}
              />} />
              <Route path="/schemas"
                element={<SchemasPage memberId={memberData?.memberId || ''} />} />
              <Route
                path="/schemas/new"
                element={
                  <SchemaRegistrationPage
                    memberId={memberData?.memberId || ''}
                  />
                }
              />
              <Route path="/schemas/logs"
                element={<Logs role="provider" memberId={memberData?.memberId || ''} />} />
              <Route path="/applications"
                element={<Applications memberId={memberData?.memberId || ''} />} />
              <Route
                path="/applications/new"
                element={
                  <ApplicationRegistration
                    memberId={memberData?.memberId || ''}
                  />
                }
              />
              <Route path="/applications/logs"
                element={<Logs role="consumer" memberId={memberData?.memberId || ''} />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </main>
        </div>
      </div>
    </Router>
  );
}

export default App;