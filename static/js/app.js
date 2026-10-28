    (() => {
        'use strict';

        const els = {
            form: document.getElementById('job-form'),
            urlInput: document.getElementById('url-input'),
            submitBtn: document.getElementById('submit-btn'),
            statusBadge: document.getElementById('status-badge'),
            jobId: document.getElementById('job-id'),
            copyJobBtn: document.getElementById('copy-job-btn'),
            uptimePill: document.getElementById('uptime-pill'),
            progressWrap: document.getElementById('progress-wrap'),
            progressFill: document.getElementById('progress-fill'),
            progressLabel: document.getElementById('progress-label'),
            progressPercent: document.getElementById('progress-percent'),
            resultBox: document.getElementById('result-box'),
            wordCount: document.getElementById('word-count'),
            resultStatus: document.getElementById('result-status'),
            resultUrl: document.getElementById('result-url'),
            logs: document.getElementById('logs'),
            clearBtn: document.getElementById('clear-btn'),
            livePill: document.getElementById('live-pill'),
            scrollTopLink: document.getElementById('scroll-top-link'),
            chips: Array.from(document.querySelectorAll('.chip')),
            stepQueued: document.getElementById('step-queued'),
            stepProcessing: document.getElementById('step-processing'),
            stepCompleted: document.getElementById('step-completed'),
        };

        const state = {
            currentJobId: null,
            currentUrl: '',
            pollTimer: null,
            pollInFlight: false,
            submitController: null,
            startedAt: null,
            lastStatus: 'IDLE',
            elapsedTimer: null,
        };

        const STATUS_MAP = {
            IDLE:       { cls: 'b-queued',     label: 'IDLE',       progress: 0,   info: 'Waiting for input' },
            QUEUED:     { cls: 'b-queued',     label: 'QUEUED',     progress: 18,  info: 'Job accepted, awaiting worker' },
            PROCESSING: { cls: 'b-processing', label: 'PROCESSING', progress: 64,  info: 'Worker actively processing' },
            COMPLETED:  { cls: 'b-completed',  label: 'COMPLETED',  progress: 100, info: 'Job finished successfully' },
            FAILED:     { cls: 'b-failed',     label: 'FAILED',     progress: 100, info: 'Job ended with an error' },
        };

        function timeStamp() {
            return new Date().toLocaleTimeString('en-GB', { hour12: false });
        }

        function setBusy(isBusy) {
            els.submitBtn.disabled = isBusy;
            els.submitBtn.textContent = isBusy ? 'Submitting…' : 'Run Job →';
            els.copyJobBtn.disabled = !state.currentJobId;
            els.livePill.style.display = state.currentJobId ? 'inline-flex' : 'none';
        }

        function setUptimeLabel(message) {
            els.uptimePill.textContent = '';
            const icon = document.createElement('span');
            icon.setAttribute('aria-hidden', 'true');
            icon.textContent = '⏱';
            const text = document.createElement('span');
            text.textContent = message;
            els.uptimePill.append(icon, text);
        }

        function setStatus(status) {
            const key = STATUS_MAP[status] ? status : 'IDLE';
            const meta = STATUS_MAP[key];
            els.statusBadge.className = `badge ${meta.cls}`;
            els.statusBadge.textContent = meta.label;
            els.progressLabel.textContent = meta.info;
            els.progressPercent.textContent = `${meta.progress}%`;
            els.progressFill.style.width = `${meta.progress}%`;

            els.stepQueued.classList.toggle('done', ['PROCESSING', 'COMPLETED', 'FAILED'].includes(key));
            els.stepProcessing.classList.toggle('active', key === 'PROCESSING');
            els.stepProcessing.classList.toggle('done', ['COMPLETED', 'FAILED'].includes(key));
            els.stepCompleted.classList.toggle('done', key === 'COMPLETED');

            els.stepQueued.classList.toggle('active', key === 'QUEUED');
            els.stepQueued.classList.toggle('done', ['PROCESSING', 'COMPLETED', 'FAILED'].includes(key));
            els.stepCompleted.classList.toggle('active', key === 'COMPLETED');

            if (key === 'IDLE') {
                els.stepQueued.classList.remove('active', 'done');
                els.stepProcessing.classList.remove('active', 'done');
                els.stepCompleted.classList.remove('active', 'done');
            }
        }

        function revealStatusShell() {
            els.progressWrap.classList.add('show');
        }

        function hideResult() {
            els.resultBox.classList.remove('show');
        }

        function showResult() {
            els.resultBox.classList.add('show');
        }

        function clearLogs() {
            els.logs.textContent = '';
            const empty = document.createElement('div');
            empty.className = 'empty-state';
            empty.id = 'empty-logs';
            empty.innerHTML = 'Waiting for jobs…<br />Submit a URL to start seeing execution events here.';
            els.logs.appendChild(empty);
        }

        function appendLog(message, type = 'info') {
            const existingEmpty = document.getElementById('empty-logs');
            if (existingEmpty) existingEmpty.remove();

            const line = document.createElement('div');
            line.className = 'log-line';

            const time = document.createElement('span');
            time.className = 'log-time';
            time.textContent = timeStamp();

            const msg = document.createElement('span');
            msg.className = `log-msg ${type}`;
            msg.textContent = String(message ?? '');

            line.append(time, msg);
            els.logs.appendChild(line);
            els.logs.scrollTop = els.logs.scrollHeight;
        }

        function normalizeUrl(input) {
            const trimmed = String(input || '').trim();
            if (!trimmed) {
                throw new Error('Please enter a URL.');
            }

            let candidate = trimmed;
            if (!/^[a-zA-Z][a-zA-Z\d+\-.]*:/.test(candidate)) {
                candidate = `https://${candidate}`;
            }

            const parsed = new URL(candidate);
            if (!['http:', 'https:'].includes(parsed.protocol)) {
                throw new Error('Only http and https URLs are supported.');
            }

            return parsed.toString();
        }

        async function readResponseJson(res) {
            const text = await res.text();
            if (!text) return {};
            try {
                return JSON.parse(text);
            } catch {
                return { raw: text };
            }
        }

        function formatJobId(id) {
            if (!id) return 'JOB · —';
            return `JOB · ${String(id).slice(0, 8).toUpperCase()}`;
        }

        async function copyText(value) {
            return navigator.clipboard.writeText(String(value));
        }

        function elapsedSeconds() {
            if (!state.startedAt) return null;
            return Math.max(0, Math.round((Date.now() - state.startedAt) / 1000));
        }

        function updateElapsedUI() {
            const secs = elapsedSeconds();
            if (secs === null) {
                setUptimeLabel('Waiting for a job');
                return;
            }

            if (state.lastStatus === 'COMPLETED') {
                setUptimeLabel(`Completed in ${secs}s`);
            } else if (state.lastStatus === 'FAILED') {
                setUptimeLabel(`Stopped after ${secs}s`);
            } else {
                setUptimeLabel(`Running for ${secs}s`);
            }
        }

        function startElapsedTicker() {
            stopElapsedTicker();
            state.elapsedTimer = window.setInterval(updateElapsedUI, 1000);
        }

        function stopElapsedTicker() {
            if (state.elapsedTimer) {
                clearInterval(state.elapsedTimer);
                state.elapsedTimer = null;
            }
        }

        function stopPolling() {
            if (state.pollTimer) {
                clearTimeout(state.pollTimer);
                state.pollTimer = null;
            }
            state.pollInFlight = false;
        }

        function schedulePoll(delay = 1800) {
            stopPolling();
            state.pollTimer = window.setTimeout(pollJob, delay);
        }

        function safeResultFromJob(job) {
            const payload = job?.result;
            if (payload == null) return {};

            if (typeof payload === 'object') return payload;

            if (typeof payload === 'string') {
                try {
                    return JSON.parse(payload);
                } catch {
                    return { raw: payload };
                }
            }

            return { raw: String(payload) };
        }

        function updateResult(job, originalUrl) {
            const result = safeResultFromJob(job);
            const wordCount = result.word_count ?? result.wordCount ?? result.words ?? '—';
            const finalUrl = result.url || originalUrl || state.currentUrl || '—';

            els.wordCount.textContent = typeof wordCount === 'number' ? wordCount.toLocaleString() : String(wordCount);
            els.resultStatus.textContent = '✓ Completed';
            els.resultUrl.textContent = finalUrl;
            els.resultUrl.href = finalUrl;
            showResult();
        }

        function setFailureResult(job) {
            els.wordCount.textContent = '—';
            els.resultStatus.textContent = job?.error ? String(job.error) : 'Job failed';
            els.resultUrl.textContent = state.currentUrl || '—';
            els.resultUrl.href = '#';
            showResult();
        }

        function markTransition(nextStatus, message, type = 'info') {
            if (state.lastStatus !== nextStatus && message) {
                appendLog(message, type);
            }
            state.lastStatus = nextStatus;
            setStatus(nextStatus);
            updateElapsedUI();
        }

        async function submitJob(rawInput) {
            const url = normalizeUrl(rawInput);

            state.currentUrl = url;
            state.lastStatus = 'QUEUED';
            state.startedAt = Date.now();

            hideResult();
            revealStatusShell();
            clearLogs();
            setBusy(true);
            setStatus('QUEUED');
            els.jobId.textContent = 'JOB · —';
            els.copyJobBtn.disabled = true;
            setUptimeLabel('Queueing job…');

            appendLog(`Submitting job for ${url}`, 'accent');

            if (state.submitController) {
                state.submitController.abort();
            }
            state.submitController = new AbortController();

            const body = {
                type: 'URL_FETCH',
                payload: JSON.stringify({ url }),
            };

            try {
                const res = await fetch('/api/jobs', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                    signal: state.submitController.signal,
                });

                const data = await readResponseJson(res);

                if (!res.ok) {
                    const message = data?.message || data?.error || `Request failed with status ${res.status}`;
                    throw new Error(message);
                }

                const jobId = data?.id || data?.jobId || data?.data?.id;
                if (!jobId) {
                    throw new Error('Server did not return a job id.');
                }

                state.currentJobId = jobId;
                els.jobId.textContent = formatJobId(jobId);
                els.copyJobBtn.disabled = false;

                appendLog(`Job created: ${jobId}`, 'info');
                appendLog('Job queued, worker will pick it up shortly', 'warn');

                markTransition('QUEUED', 'Job accepted by API', 'info');
                startElapsedTicker();
                schedulePoll(800);
                loadRecentJobs();
            } catch (error) {
                const message = error?.name === 'AbortError'
                    ? 'Previous request cancelled.'
                    : (error?.message || 'Unknown submission error');

                appendLog(`Error: ${message}`, 'error');
                setStatus('FAILED');
                state.lastStatus = 'FAILED';
                setUptimeLabel('Submission failed');
                setBusy(false);
                stopPolling();
                stopElapsedTicker();
            }
        }

        async function pollJob() {
            if (!state.currentJobId || state.pollInFlight) {
                return;
            }

            state.pollInFlight = true;

            try {
                const res = await fetch(`/api/jobs/${encodeURIComponent(state.currentJobId)}`, {
                    headers: { 'Accept': 'application/json' },
                });

                const job = await readResponseJson(res);

                if (!res.ok) {
                    throw new Error(job?.message || job?.error || `Polling failed with status ${res.status}`);
                }

                const status = String(job?.status || 'QUEUED').toUpperCase();

                if (status === 'PROCESSING') {
                    setStatus('PROCESSING');
                    state.lastStatus = 'PROCESSING';
                    updateElapsedUI();
                    appendLog('Worker is processing the job', 'accent');
                } else if (status === 'QUEUED') {
                    setStatus('QUEUED');
                    state.lastStatus = 'QUEUED';
                    updateElapsedUI();
                    appendLog('Job still waiting in the queue', 'warn');
                } else if (status === 'COMPLETED') {
                    stopPolling();
                    markTransition('COMPLETED', 'Job completed successfully', 'success');
                    updateResult(job, state.currentUrl);
                    setBusy(false);
                    updateElapsedUI();
                    appendLog(`Word count: ${safeResultFromJob(job).word_count ?? safeResultFromJob(job).wordCount ?? '—'}`, 'success');
                    loadRecentJobs();
                    return;
                } else if (status === 'FAILED') {
                    stopPolling();
                    markTransition('FAILED', `Job failed: ${job?.error || 'unknown error'}`, 'error');
                    setFailureResult(job);
                    setBusy(false);
                    updateElapsedUI();
                    loadRecentJobs();
                    return;
                } else {
                    markTransition(status, `Observed status: ${status}`, 'info');
                }

                schedulePoll(1800);
            } catch (error) {
                appendLog(`Polling error: ${error?.message || 'Unknown error'}`, 'error');
                schedulePoll(2500);
            } finally {
                state.pollInFlight = false;
            }
        }

        function resetUi() {
            stopPolling();
            stopElapsedTicker();
            state.currentJobId = null;
            state.currentUrl = '';
            state.startedAt = null;
            state.lastStatus = 'IDLE';
            state.submitController = null;

            els.jobId.textContent = 'JOB · —';
            els.copyJobBtn.disabled = true;
            els.resultUrl.textContent = '—';
            els.resultUrl.href = '#';
            els.wordCount.textContent = '—';
            els.resultStatus.textContent = '—';
            setStatus('IDLE');
            setUptimeLabel('Waiting for a job');
            els.progressWrap.classList.remove('show');
            hideResult();
            setBusy(false);
        }

        els.form.addEventListener('submit', async (event) => {
            event.preventDefault();
            await submitJob(els.urlInput.value);
        });

        els.clearBtn.addEventListener('click', () => {
            clearLogs();
        });

        els.copyJobBtn.addEventListener('click', async () => {
            if (!state.currentJobId) return;
            try {
                await copyText(state.currentJobId);
                appendLog('Job ID copied to clipboard', 'success');
            } catch {
                appendLog('Could not copy job ID', 'warn');
            }
        });

        els.chips.forEach((chip) => {
            chip.addEventListener('click', () => {
                const url = chip.getAttribute('data-url');
                if (url) {
                    els.urlInput.value = url;
                    els.urlInput.focus();
                    els.urlInput.setSelectionRange(url.length, url.length);
                }
            });
        });

        els.scrollTopLink.addEventListener('click', (event) => {
            event.preventDefault();
            window.scrollTo({ top: 0, behavior: 'smooth' });
        });

        window.addEventListener('beforeunload', () => {
            stopPolling();
            stopElapsedTicker();
        });

        document.addEventListener('visibilitychange', () => {
            if (document.hidden) return;
            if (state.currentJobId && !state.pollTimer && state.lastStatus !== 'COMPLETED' && state.lastStatus !== 'FAILED') {
                schedulePoll(300);
            }
        });

        function escapeHtml(str) {
            return String(str ?? '')
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#39;');
        }

        async function loadRecentJobs() {
            try {
                const res = await fetch('/api/jobs?limit=8');
                if (!res.ok) return;
                const data = await res.json();
                const tbody = document.getElementById('recent-jobs-body');
                if (!tbody) return;
                if (!data.jobs || data.jobs.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;padding:24px;color:var(--text-3);">No jobs recorded yet.</td></tr>';
                    return;
                }
                tbody.innerHTML = data.jobs.map(j => {
                    let parsed = {};
                    try { parsed = JSON.parse(j.payload || '{}'); } catch(e) {}
                    let resObj = {};
                    try { resObj = JSON.parse(j.result || '{}'); } catch(e) {}
                    const wc = resObj.word_count != null ? escapeHtml(resObj.word_count.toLocaleString()) : (j.error ? 'Error' : '—');
                    const statusCls = j.status === 'COMPLETED' ? 'b-completed' : (j.status === 'FAILED' ? 'b-failed' : (j.status === 'PROCESSING' ? 'b-processing' : 'b-queued'));
                    const safeId = escapeHtml(j.id || '');
                    const shortId = escapeHtml((j.id || '').slice(0, 8));
                    const safeStatus = escapeHtml(j.status || 'QUEUED');
                    const safeUrl = escapeHtml(parsed.url || '—');
                    const safeAttempts = `${Number(j.attempts) || 0}/${Number(j.max_retries) || 0}`;
                    const timeStr = j.created_at ? escapeHtml(new Date(j.created_at).toLocaleTimeString()) : '—';
                    return `
                        <tr data-id="${safeId}" class="recent-job-row" style="cursor:pointer;" title="Click to view details">
                            <td><span class="badge ${statusCls}" style="padding:4px 9px;font-size:0.72rem;">${safeStatus}</span></td>
                            <td style="font-family:var(--mono);color:var(--accent-2);">${shortId}</td>
                            <td style="max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${safeUrl}</td>
                            <td style="font-weight:700;">${wc}</td>
                            <td>${safeAttempts}</td>
                            <td style="color:var(--text-3);font-size:0.78rem;">${timeStr}</td>
                        </tr>
                    `;
                }).join('');

                document.querySelectorAll('.recent-job-row').forEach(row => {
                    row.addEventListener('click', async () => {
                        const id = row.getAttribute('data-id');
                        if (id) {
                            await inspectJob(id);
                        }
                    });
                });
            } catch(err) {
                console.error('Failed to load recent jobs:', err);
            }
        }

        async function inspectJob(jobId) {
            try {
                stopPolling();
                stopElapsedTicker();
                const res = await fetch(`/api/jobs/${encodeURIComponent(jobId)}`);
                if (!res.ok) return;
                const job = await res.json();
                state.currentJobId = job.id;
                state.lastStatus = job.status;
                els.jobId.textContent = formatJobId(job.id);
                els.copyJobBtn.disabled = false;
                revealStatusShell();
                setStatus(job.status);
                let parsed = {};
                try { parsed = JSON.parse(job.payload || '{}'); } catch(e) {}
                state.currentUrl = parsed.url || '';
                if (job.status === 'COMPLETED') {
                    setSubmitting(false);
                    updateResult(job, parsed.url);
                } else if (job.status === 'FAILED') {
                    setSubmitting(false);
                    setFailureResult(job);
                } else {
                    hideResult();
                    state.startedAt = job.created_at ? new Date(job.created_at).getTime() : Date.now();
                    startElapsedTicker();
                    schedulePoll(300);
                }
                appendLog(`Inspecting job: ${job.id} (${job.status})`, 'info');
                window.scrollTo({ top: 180, behavior: 'smooth' });
            } catch(err) {
                console.error('Failed to inspect job:', err);
            }
        }

        const refreshHistoryBtn = document.getElementById('refresh-history-btn');
        if (refreshHistoryBtn) {
            refreshHistoryBtn.addEventListener('click', () => {
                loadRecentJobs();
            });
        }

        resetUi();
        clearLogs();
        loadRecentJobs();
    })();
