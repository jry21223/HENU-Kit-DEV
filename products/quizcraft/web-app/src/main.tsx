import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Layout from '@/components/Layout';
import Practice from '@/pages/Practice';
import Quiz from '@/pages/Quiz';
import Result from '@/pages/Result';
import Ranking from '@/pages/Ranking';
import Feedback from '@/pages/Feedback';
import Favorites from '@/pages/Favorites';
import { QUIZCRAFT_GO_SHADOW_ENABLED } from '@/api/quizcraftShadowClient';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter basename={import.meta.env.BASE_URL}>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Navigate to="/practice" replace />} />
          <Route path="practice" element={<Practice />} />
          <Route path="quiz" element={<Quiz />} />
          <Route path="result" element={<Result />} />
          <Route path="favorites" element={QUIZCRAFT_GO_SHADOW_ENABLED ? <Favorites /> : <Navigate to="/practice" replace />} />
          <Route path="ranking" element={<Ranking />} />
          <Route path="feedback" element={<Feedback />} />
          <Route path="feedback-board" element={<Navigate to="/feedback" replace />} />
          <Route path="workshop/feedback/:feedbackId" element={<Navigate to="/feedback" replace />} />
          <Route path="wheel" element={<Navigate to="/practice" replace />} />
          <Route path="extract" element={<Navigate to="/practice" replace />} />
          <Route path="*" element={<Navigate to="/practice" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  </React.StrictMode>
);
