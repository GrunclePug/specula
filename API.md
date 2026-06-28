# Specula JSON API Documentation

Specula provides a simple JSON API to query the status of monitored targets programmatically. 

---

## 1. Retrieve Statuses

### `GET /api/v1/status`

Retrieves the real-time status of targets. By default, results are sorted naturally by IP address ascending.

#### Query Parameters

All parameters are optional and can be combined to narrow down results.

| Parameter | Type   | Description | Example |
| :--- | :--- | :--- | :--- |
| `group`   | String | Filter by a specific group (exact match) | `?group=public` |
| `host`    | String | Filter by host IP address or domain (case-insensitive partial match) | `?host=192.168` |
| `ip`      | String | Alias for the `host` parameter | `?ip=8.8` |
| `name`    | String | Filter by friendly name label (case-insensitive partial match) | `?name=dns` |

---

## 2. Examples

### Query All Targets
```bash
curl http://localhost:8080/api/v1/status
```

### Query by Group
Matches targets that belong to the specified group (exact name check).
```bash
curl http://localhost:8080/api/v1/status?group=infrastructure
```

### Query by Host / IP (Partial Match)
Matches targets whose IP address or domain contains the query substring.
```bash
curl http://localhost:8080/api/v1/status?ip=192.168.1
```

### Query by Name (Partial Match)
Matches targets whose friendly label contains the query substring.
```bash
curl http://localhost:8080/api/v1/status?name=gateway
```

### Combined Filters
Retrieve public targets that have "Google" in their name:
```bash
curl "http://localhost:8080/api/v1/status?group=public&name=google"
```

---

## 3. JSON Response Schema

#### Success Response (`200 OK`)

```json
[
  {
    "host": "8.8.8.8",
    "name": "Google DNS",
    "groups": [
      "dns",
      "public"
    ],
    "up": true,
    "loss": 0,
    "status": "healthy",
    "rtt": 12543000,
    "last_check": "2026-06-26T17:55:00-07:00"
  }
]
```

#### Fields Description

* **`host`** *(string)*: Target IP address, domain name, or `host:port` combination.
* **`name`** *(string)*: Friendly label assigned to the target.
* **`groups`** *(array of strings)*: List of groups associated with this target.
* **`up`** *(boolean)*: `true` if the last ping check succeeded with packet loss under 100%.
* **`loss`** *(float)*: Percentage of packet loss (from `0` to `100`).
* **`status`** *(string)*: Categorized status based on loss:
  * `"healthy"`: 0% packet loss.
  * `"degraded"`: > 0% and < 100% packet loss.
  * `"offline"`: 100% packet loss.
  * `"pending"`: Target registered but check has not run yet.
* **`rtt`** *(integer)*: Average round-trip time latency in nanoseconds (`1000000` ns = `1` ms).
* **`last_check`** *(string)*: Timestamp of the last executed ping check (RFC3339 format).
