# Observability assets

## Prometheus

Scrape `/api/v1/metrics` with an API key:

```yaml
scrape_configs:
  - job_name: backup-manager
    metrics_path: /api/v1/metrics
    static_configs:
      - targets: ["backup-manager:8080"]
    authorization:
      type: APIKey
      credentials: <admin-key>
```

Load `prometheus-rules.yml` into Prometheus:

```yaml
rule_files:
  - /etc/prometheus/rules/backup-manager.yml
```

## Grafana

Import `grafana-dashboard.json` and select your Prometheus datasource. The
dashboard covers backups, failures, overdue profiles, queue depth, restore
status and HTTP latency.

## Notes

- The metrics endpoint is authenticated; use the `ADMIN_KEY` or a dedicated
  API key.
- Profile labels contain the profile ID and name. Renaming a profile creates a
  new time series; this is expected.
- The service is single-instance; metrics from multiple replicas would be
  duplicated.
