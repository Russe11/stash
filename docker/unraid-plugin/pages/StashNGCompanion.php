<?php
$pluginRoot = dirname(__DIR__);
$auditHelper = $pluginRoot . '/helpers/stash-ng-audit.sh';
$report = '';
$error = '';

if ($_SERVER['REQUEST_METHOD'] === 'POST' && ($_POST['action'] ?? '') === 'run_audit') {
    $cmd = escapeshellarg($auditHelper) . ' --redacted';
    $output = [];
    $exitCode = 0;
    exec($cmd . ' 2>&1', $output, $exitCode);
    $report = implode("\n", $output);
    if ($exitCode !== 0) {
        $error = 'Audit helper failed with exit code ' . (int)$exitCode;
    }
}
?>
<div class="stash-ng-companion">
  <h2>Stash NG Companion</h2>
  <p>Runs a local-only audit of the Stash NG Unraid deployment.</p>
  <form method="post">
    <input type="hidden" name="action" value="run_audit">
    <button type="submit">Run Audit</button>
  </form>
  <?php if ($error !== ''): ?>
    <p class="error"><?= htmlspecialchars($error, ENT_QUOTES, 'UTF-8') ?></p>
  <?php endif; ?>
  <?php if ($report !== ''): ?>
    <h3>Redacted Audit Report</h3>
    <pre><?= htmlspecialchars($report, ENT_QUOTES, 'UTF-8') ?></pre>
  <?php endif; ?>
</div>
