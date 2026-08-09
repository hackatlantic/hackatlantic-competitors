# Service objectives

| Objective | Target | Measurement |
| --- | --- | --- |
| API availability | 99.9% rolling 30 days | Successful synthetic `/readyz` checks and non-5xx request ratio |
| Scanner lookup p95 | 500 ms or less | API duration histogram by route template |
| Redemption p95 | 750 ms or less at 25 concurrent scanners | Staging k6 gate and production histogram |
| Load-test error rate | Below 1% | k6 `http_req_failed` |
| Recovery point | 24 hours or less | Timestamp of latest verified encrypted backup |
| Recovery time | 30 minutes or less | Monthly restore-drill duration |
| API rollback | 5 minutes or less | Failed-smoke to healthy prior digest |

The 99.9% availability objective allows approximately 43 minutes of unavailability over 30 days. Alerts page administrators only after three consecutive synthetic failures, five minutes above 5% server errors, a scanner latency violation, deployment failure, or backup/restore failure.

Targets are initial objectives, not claims of achieved performance. Replace the baseline and final values only after reports have been captured.
