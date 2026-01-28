# PromQL Queries for Monitoring Valkey Cluster Connections

## Setting Up Grafana

### 1. Access Grafana

Grafana is available at `http://localhost:3000` (when running via docker-compose).

Default credentials:
- **Username**: `admin`
- **Password**: `admin`

You will be prompted to change the password on first login.

### 2. Add Prometheus Data Source

1. Navigate to **Configuration** → **Data Sources** (or go to `http://localhost:3000/datasources`)
2. Click **Add data source**
3. Select **Prometheus**
4. Configure the data source:
   - **Name**: `Prometheus` (or any name you prefer)
   - **URL**: `http://prometheus:9090` (internal Docker network) or `http://localhost:9090` (if accessing from host)
   - **Access**: Server (default)
5. Click **Save & Test** to verify the connection

### 3. Create a Dashboard

1. Navigate to **Dashboards** → **New Dashboard** (or go to `http://localhost:3000/dashboard/new`)
2. Click **Add visualization** to create your first panel
3. Select the Prometheus data source you just created
4. Use the PromQL queries from the sections below

### 4. Recommended Dashboard Panels

#### Panel 1: Total Connections (Stat Panel)
- **Title**: Total Connected Clients
- **Query**: `sum(redis_connected_clients{job="valkey_exporters"})`
- **Panel Type**: Stat
- **Unit**: Short

#### Panel 2: Connections per Shard (Time Series)
- **Title**: Connections by Shard
- **Query**: `redis_connected_clients{job="valkey_exporters"}`
- **Panel Type**: Time series
- **Legend**: `{{shard}}` or `{{instance}}`
- **Unit**: Short

#### Panel 3: Rejected Connections (Time Series)
- **Title**: Rejected Connections Rate
- **Query**: `rate(redis_rejected_connections_total{job="valkey_exporters"}[5m])`
- **Panel Type**: Time series
- **Legend**: `{{shard}}`
- **Unit**: ops/sec

#### Panel 4: Cluster State (Stat Panel)
- **Title**: Cluster State
- **Query**: `redis_cluster_state{job="valkey_exporters"}`
- **Panel Type**: Stat
- **Value Mappings**: 
  - `1` → `OK` (green)
  - `0` → `Not OK` (red)

#### Panel 5: Cluster Nodes (Stat Panel)
- **Title**: Connected Cluster Nodes
- **Query**: `redis_cluster_connected_nodes{job="valkey_exporters"}`
- **Panel Type**: Stat
- **Unit**: Short

#### Panel 6: Cluster Slots Status (Time Series)
- **Title**: Cluster Slots Status
- **Queries**:
  - `redis_cluster_slots_assigned{job="valkey_exporters"}` (label: "Assigned")
  - `redis_cluster_slots_ok{job="valkey_exporters"}` (label: "OK")
- **Panel Type**: Time series
- **Unit**: Short

### 5. Dashboard JSON Export

You can export your dashboard configuration as JSON for sharing or backup:
1. Go to your dashboard
2. Click the **Settings** icon (gear) in the top right
3. Click **JSON Model**
4. Copy the JSON configuration

### 6. Import Pre-configured Dashboard (Optional)

If you have a dashboard JSON file:
1. Navigate to **Dashboards** → **Import**
2. Paste the JSON or upload the file
3. Select your Prometheus data source
4. Click **Import**`