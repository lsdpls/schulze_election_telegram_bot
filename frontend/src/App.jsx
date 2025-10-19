import React, { useState, useEffect } from 'react';
import axios from 'axios';
import './App.css';

function App() {
  const [votes, setVotes] = useState([]);
  const [candidates, setCandidates] = useState({});
  const [results, setResults] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    loadData();
  }, []);

  const getInitials = (text) => {
    return text.split(' ').map(word => word[0]).join('');
  };

  const MatrixTable = ({ data, type = 'preferences', candidates: candidatesData }) => {
    const [hoveredCell, setHoveredCell] = useState(null);
    
    try {
      const matrix = typeof data === 'string' ? JSON.parse(data) : data;
      const candidates = Object.keys(matrix).sort((a, b) => parseInt(a) - parseInt(b));
      
      // Функция для определения, нужно ли выделить ячейку
      const shouldHighlight = (rowCandidate, colCandidate) => {
        if (rowCandidate === colCandidate) return false;
        
        if (type === 'preferences') {
          // В preferences выделяем ячейки где A > B (прямое предпочтение)
          const directPreference = matrix[rowCandidate] && matrix[rowCandidate][colCandidate] || 0;
          const reversePreference = matrix[colCandidate] && matrix[colCandidate][rowCandidate] || 0;
          return directPreference > reversePreference;
        } else if (type === 'strongest_paths') {
          // В strongest paths выделяем ячейки где A > B (парные победы)
          const directPath = matrix[rowCandidate] && matrix[rowCandidate][colCandidate] || 0;
          const reversePath = matrix[colCandidate] && matrix[colCandidate][rowCandidate] || 0;
          return directPath > reversePath;
        }
        return false;
      };

      // Функция для определения, является ли ячейка частью пары при hover
      const isHoveredPair = (rowCandidate, colCandidate) => {
        if (!hoveredCell) return false;
        const [hoverRow, hoverCol] = hoveredCell;
        return (rowCandidate === hoverRow && colCandidate === hoverCol) ||
               (rowCandidate === hoverCol && colCandidate === hoverRow);
      };
      
      return (
        <div className="matrix-table">
          <table>
            <thead>
              <tr>
                <th></th>
                {candidates.map(candidate => (
                  <th key={candidate}>{candidatesData[candidate]?.name || `К${candidate}`}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {candidates.map(rowCandidate => (
                <tr key={rowCandidate}>
                  <td className="matrix-label">{candidatesData[rowCandidate]?.name || `К${rowCandidate}`}</td>
                  {candidates.map(colCandidate => (
                    <td 
                      key={colCandidate} 
                      className={`matrix-cell ${shouldHighlight(rowCandidate, colCandidate) ? 'matrix-highlight' : ''} ${isHoveredPair(rowCandidate, colCandidate) ? 'matrix-hover-pair' : ''}`}
                      onMouseEnter={() => setHoveredCell([rowCandidate, colCandidate])}
                      onMouseLeave={() => setHoveredCell(null)}
                    >
                      {matrix[rowCandidate] && matrix[rowCandidate][colCandidate] !== undefined 
                        ? matrix[rowCandidate][colCandidate] 
                        : '-'}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    } catch (error) {
      return <div className="matrix-error">Ошибка отображения матрицы</div>;
    }
  };

  const loadData = async () => {
    try {
      const [votesRes, candidatesRes, resultsRes] = await Promise.all([
        axios.get('/election_bot/votes'),
        axios.get('/election_bot/candidates'),
        axios.get('/election_bot/result')
      ]);
      
      const sortedVotes = votesRes.data.sort((a, b) => 
        new Date(a.created_at) - new Date(b.created_at)
      );
      setVotes(sortedVotes);
      
      const candidatesMap = {};
      candidatesRes.data.forEach(c => {
        candidatesMap[c.candidate_id] = {
          name: getInitials(c.name),
          course: getInitials(c.course)
        };
      });
      setCandidates(candidatesMap);
      
      const sortedResults = resultsRes.data.sort((a, b) => a.course.localeCompare(b.course));
      setResults(sortedResults);
      
      setLoading(false);
    } catch (err) {
      setError(err.message);
      setLoading(false);
    }
  };


  if (loading) {
    return (
      <div className="container">
        <div className="loading">Загрузка...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="container">
        <div className="error">❌ Ошибка загрузки данных: {error}</div>
      </div>
    );
  }

  return (
    <div className="container">
      <div className="floating-menu">
        <div className="floating-item">
          <span className="floating-label">Код</span>
          <a href="https://github.com/lsdpls/schulze_election_telegram_bot" target="_blank" rel="noopener noreferrer" className="floating-link" title="Исходный код">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12,2A10,10 0 0,0 2,12C2,16.42 4.87,20.17 8.84,21.5C9.34,21.58 9.5,21.27 9.5,21C9.5,20.77 9.5,20.14 9.5,19.31C6.73,19.91 6.14,17.97 6.14,17.97C5.68,16.81 5.03,16.5 5.03,16.5C4.12,15.88 5.1,15.9 5.1,15.9C6.1,15.97 6.63,16.93 6.63,16.93C7.5,18.45 8.97,18 9.54,17.76C9.63,17.11 9.89,16.67 10.17,16.42C7.95,16.17 5.62,15.31 5.62,11.5C5.62,10.39 6,9.5 6.65,8.79C6.55,8.54 6.2,7.5 6.75,6.15C6.75,6.15 7.59,5.88 9.5,7.17C10.29,6.95 11.15,6.84 12,6.84C12.85,6.84 13.71,6.95 14.5,7.17C16.41,5.88 17.25,6.15 17.25,6.15C17.8,7.5 17.45,8.54 17.35,8.79C18,9.5 18.38,10.39 18.38,11.5C18.38,15.32 16.04,16.16 13.81,16.41C14.17,16.72 14.5,17.33 14.5,18.26C14.5,19.6 14.5,20.68 14.5,21C14.5,21.27 14.66,21.59 15.17,21.5C19.14,20.16 22,16.42 22,12A10,10 0 0,0 12,2Z"/>
            </svg>
          </a>
        </div>
        <div className="floating-item">
          <span className="floating-label">Алгоритм</span>
          <a href="https://en.wikipedia.org/wiki/Schulze_method" target="_blank" rel="noopener noreferrer" className="floating-link" title="Метод Шульце">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M11,19.93C7.05,19.44 4,16.08 4,12C4,11.38 4.08,10.79 4.21,10.21L9,15V16A2,2 0 0,0 11,18M18.9,17.39C18.64,16.58 17.9,16 17,16H16V13A1,1 0 0,0 15,12H8V10H10A1,1 0 0,0 11,9V7H13A2,2 0 0,0 15,5V4.59C17.93,5.78 20,8.65 20,12A7.74,7.74 0 0,1 18.9,17.39Z"/>
            </svg>
          </a>
        </div>
        <div className="floating-item">
          <span className="floating-label">Положение</span>
          <a href="https://studsovet.spbu.ru/images/polozheniya/polozhenie_pmpu.pdf" target="_blank" rel="noopener noreferrer" className="floating-link" title="Положение о студенческом совете">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M21,4H3A2,2 0 0,0 1,6V19A2,2 0 0,0 3,21H21A2,2 0 0,0 23,19V6A2,2 0 0,0 21,4M21,19H12V6H21V19M3,19V6H10V19H3Z"/>
            </svg>
          </a>
        </div>
      </div>
      <h1>
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor" style={{marginRight: '8px', verticalAlign: 'middle'}}>
          <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-5 14H7v-2h7v2zm3-4H7v-2h10v2zm0-4H7V7h10v2z"/>
        </svg>
        Результаты голосования
      </h1>

      <div className="stats">
        <div className="stat-card">
          <div className="stat-number">{votes.length}</div>
          <div className="stat-label">Всего голосов</div>
        </div>
        {results.length > 0 && (
          <a href="#results" className="stat-card results-link">
            <div className="stat-number">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
                <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zM9 17H7v-7h2v7zm4 0h-2V7h2v10zm4 0h-2v-4h2v4z"/>
              </svg>
            </div>
            <div className="stat-label">Результаты</div>
          </a>
        )}
      </div>

      {votes.length === 0 ? (
        <div className="no-results">Голоса не найдены</div>
      ) : (
        <div className="votes-container">
          <div className="votes-header">
            <div className="header-token">Токен голоса</div>
            <div className="header-rankings">
              {(() => {
                const candidatesCount = Object.keys(candidates).length;
                const voteLength = votes.length > 0 ? votes[0].candidate_rankings.length : 0;
                const headerCount = candidatesCount > 0 ? candidatesCount : voteLength;
                
                return headerCount > 0 && Array.from({ length: headerCount }, (_, index) => (
                  <div key={index + 1} className="header-position">{index + 1}</div>
                ));
              })()}
            </div>
          </div>
          <div className="votes-list">
            {votes.map((vote, index) => (
              <div key={index} className="vote-row">
                <div className="vote-token-cell">{vote.vote_token}</div>
                <div className="vote-rankings-cell">
                  {vote.candidate_rankings.map((candidateId, idx) => {
                    const candidate = candidates[candidateId];
                    return (
                      <div key={idx} className="candidate-badge">
                        <span className="name">{candidate?.name || `#${candidateId}`}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {results.length > 0 && (
        <div id="results" className="results-section">
          <h2>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" style={{marginRight: '8px', verticalAlign: 'middle'}}>
              <path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zM9 17H7v-7h2v7zm4 0h-2V7h2v10zm4 0h-2v-4h2v4z"/>
            </svg>
            Результаты выборов
          </h2>
          <div className="results-list">
            {results.map((result, index) => (
              <div key={index} className="result-card">
                <div className="result-header">
                  <div className="result-info-line">
                    <div className="info-item">
                      <span className="info-value">{result.course}</span>
                    </div>
                    <div className="info-item">
                      <span className="info-label">Победители:</span>
                      <div className="winners-list">
                        {result.winner_candidate_id.map((candidateId, idx) => (
                          <span key={idx} className="winner-badge">
                            {candidates[candidateId]?.name || `#${candidateId}`}
                          </span>
                        ))}
                      </div>
                    </div>
                    <div className="info-item">
                      <span className="result-stage">{result.stage}</span>
                    </div>
                  </div>
                </div>

                {result.preferences && (
                  <div className="result-matrix">
                    <h4>Парные предпочтения:</h4>
                    <div className="matrix-container">
                      <MatrixTable data={result.preferences} type="preferences" candidates={candidates} />
                    </div>
                  </div>
                )}

                {result.strongest_paths && (
                  <div className="result-matrix">
                    <h4>Сильнейшие пути:</h4>
                    <div className="matrix-container">
                      <MatrixTable data={result.strongest_paths} type="strongest_paths" candidates={candidates} />
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export default App;

