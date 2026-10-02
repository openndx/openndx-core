import { Route, Routes } from 'react-router-dom';
import ConsentDetailPage from "./pages/ConsentDetailPage.tsx";
import ConsentListPage from "./pages/ConsentListPage.tsx";
import ConsentPage from "./pages/ConsentPage.tsx";
import ErrorPage from "./pages/ErrorPage.tsx";
import LoginPage from "./pages/LoginPage.tsx";
import SuccessPage from "./pages/SuccessPage.tsx";
import UnauthorizedPage from "./pages/UnauthorizedPage.tsx";

function App() {
  return (
    <Routes>
      <Route path="/" element={<ConsentPage />} />
      <Route path="/consents" element={<ConsentListPage />} />
      <Route path="/consents/:consentId" element={<ConsentDetailPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/error" element={<ErrorPage />} />
      <Route path="/success" element={<SuccessPage />} />
      <Route path="/unauthorized" element={<UnauthorizedPage />} />
    </Routes>
  )
}

export default App;
