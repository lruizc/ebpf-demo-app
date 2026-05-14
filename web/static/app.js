(function () {
  const btn       = document.getElementById('query-btn');
  const select    = document.getElementById('city-select');
  const result    = document.getElementById('result-panel');
  const errPanel  = document.getElementById('error-panel');
  const badge     = document.getElementById('source-badge');
  const cityName  = document.getElementById('city-name');
  const tempEl    = document.getElementById('temperature');
  const condEl    = document.getElementById('condition');
  const fetchedEl = document.getElementById('fetched-at');
  const errMsg    = document.getElementById('error-message');

  const SOURCE_MAP = {
    cache:  { text: 'cache HIT (internal)',     cls: 'badge--internal' },
    origin: { text: 'cache MISS (external)',    cls: 'badge--external' },
    bypass: { text: 'BYPASS (always external)', cls: 'badge--bypass'   },
  };

  function showError(msg) {
    result.classList.add('hidden');
    errMsg.textContent = 'Error: ' + msg;
    errPanel.classList.remove('hidden');
  }

  function showResult(data) {
    errPanel.classList.add('hidden');
    const src = SOURCE_MAP[data.source] || { text: data.source, cls: 'badge--external' };
    badge.className = 'badge ' + src.cls;
    badge.textContent = src.text;
    cityName.textContent  = data.display_name;
    tempEl.textContent    = data.temperature_c.toFixed(1) + ' °C';
    condEl.textContent    = data.condition;
    fetchedEl.textContent = 'Fetched at: ' + data.fetched_at;
    result.classList.remove('hidden');
  }

  btn.addEventListener('click', function () {
    const city = select.value;
    btn.disabled = true;
    fetch('/api/weather?city=' + encodeURIComponent(city))
      .then(function (res) {
        return res.json().then(function (body) {
          if (!res.ok) { throw new Error(body.message || 'HTTP ' + res.status); }
          return body;
        });
      })
      .then(showResult)
      .catch(function (err) { showError(err.message); })
      .finally(function () { btn.disabled = false; });
  });
}());
