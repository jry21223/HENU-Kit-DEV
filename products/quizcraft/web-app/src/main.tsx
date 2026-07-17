import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Layout from '@/components/Layout';
import Home from '@/pages/Home';
import Practice from '@/pages/Practice';
import Quiz from '@/pages/Quiz';
import Result from '@/pages/Result';
import Ranking from '@/pages/Ranking';
import Extract from '@/pages/Extract';
import Feedback from '@/pages/Feedback';
import FeedbackBoard from '@/pages/FeedbackBoard';
import FoodWheel from '@/pages/FoodWheel';
import { IS_OPS_MODE } from '@/config/appMode';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter basename={import.meta.env.BASE_URL}>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route
            index
            element={IS_OPS_MODE ? <Navigate to="/practice" replace /> : <Home />}
          />
          <Route path="practice" element={<Practice />} />
          <Route path="quiz" element={<Quiz />} />
          <Route path="result" element={<Result />} />
          <Route path="ranking" element={<Ranking />} />
          <Route path="feedback" element={<Feedback />} />
          <Route path="feedback-board" element={<FeedbackBoard />} />
          <Route
            path="wheel"
            element={IS_OPS_MODE ? <Navigate to="/practice" replace /> : <FoodWheel />}
          />
          <Route
            path="extract"
            element={IS_OPS_MODE ? <Navigate to="/practice" replace /> : <Extract />}
          />
          <Route path="*" element={<Navigate to="/practice" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  </React.StrictMode>
);
