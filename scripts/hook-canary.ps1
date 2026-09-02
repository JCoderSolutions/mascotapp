#Requires -Version 5.1
<#
    PreToolUse canary hook  --  task T-00-018

    PURPOSE
    Answer two questions that the Claude Code documentation does NOT answer,
    by observation instead of assumption:

      Q1. Does a PreToolUse hook fire at all when the session runs in
          `bypassPermissions` mode?
      Q2. If it fires, does `exit 2` actually BLOCK the tool call in that mode?

    Until both are answered YES, the hook layer (layer 4 of the supervision
    model in the master plan, section 7.3) counts as NOTHING. A barrier that was
    never tested is not a barrier.

    HOW IT ANSWERS THEM
    Every invocation appends a FIRED line to the log. That answers Q1.
    A command containing the marker GENTLE_CANARY_PROBE is refused with
    exit code 2. That answers Q2.

    Exit code semantics, verified against the Claude Code docs:
      exit 0 -> allow, hook output is not surfaced
      exit 1 -> NON-blocking error. The tool call PROCEEDS. Never use it to deny.
      exit 2 -> blocking error. stderr is fed back to the model.

    INSTALL  (the user must do this by hand: the agent is denied write access to
    .claude/settings.json on purpose, so it cannot rewrite its own guardrails)

    Add to .claude/settings.json:

      "hooks": {
        "PreToolUse": [
          {
            "matcher": "Bash",
            "hooks": [
              {
                "type": "command",
                "command": "powershell -NoProfile -ExecutionPolicy Bypass -File .claude/../scripts/hook-canary.ps1"
              }
            ]
          }
        ]
      }

    RUN THE PROBE

      1. echo GENTLE_CANARY_PROBE

         Blocked  -> Q1 yes, Q2 yes. The hook layer is real.
         Executed -> read the log:
                       FIRED line present -> Q1 yes, Q2 NO. The hook observes
                                             but cannot deny. Logging only.
                       no FIRED line      -> Q1 no. The layer does not exist in
                                             this mode. Strike it from the model.

      2. Record the observed result in
         docs/vault/20-arquitectura/supervision-checks.md
         and update the plan's layer-4 row to match what was SEEN.

    UNINSTALL
    Remove the "hooks" block above. This script writes nothing but its own log.
#>

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$logPath  = Join-Path $repoRoot 'docs/vault/20-arquitectura/hook-canary.log'

$raw = [Console]::In.ReadToEnd()

$payload = $null
try { if ($raw) { $payload = $raw | ConvertFrom-Json } } catch { $payload = $null }

$toolName = '<unparsed>'
$command  = ''
if ($payload) {
    if ($payload.PSObject.Properties.Name -contains 'tool_name') {
        $toolName = [string]$payload.tool_name
    }
    if ($payload.PSObject.Properties.Name -contains 'tool_input' -and $payload.tool_input) {
        if ($payload.tool_input.PSObject.Properties.Name -contains 'command') {
            $command = [string]$payload.tool_input.command
        }
    }
}

$stamp = (Get-Date).ToString('o')

# One line per invocation. Its mere presence is the answer to Q1.
Add-Content -LiteralPath $logPath -Encoding UTF8 -Value "$stamp`tFIRED`ttool=$toolName`tcommand=$command"

if ($command -match 'GENTLE_CANARY_PROBE') {
    Add-Content -LiteralPath $logPath -Encoding UTF8 -Value "$stamp`tDENY_ATTEMPTED`ttool=$toolName"
    [Console]::Error.WriteLine('hook-canary: denied GENTLE_CANARY_PROBE (T-00-018 probe). If you are reading this as a tool error, exit 2 blocks in this permission mode.')
    exit 2
}

exit 0
