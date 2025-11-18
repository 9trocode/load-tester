# Database Persistence Guide

This guide explains how to persist the SQLite database in PipeOps Load Tester across container restarts and deployments.

## 📍 Database Location

The SQLite database is stored at:

```
./data/loadtest.db
```

SQLite with WAL (Write-Ahead Logging) mode creates additional files:

- `loadtest.db` - Main database file
- `loadtest.db-shm` - Shared memory file
- `loadtest.db-wal` - Write-ahead log file

All these files are stored in the `./data` directory.

---

## Docker Volume Mounting

### Using Docker Compose (Recommended)

The included `docker-compose.yml` already has the volume configured:

```yaml
services:
  load-tester:
    volumes:
      - load-tester-data:/home/pipeops/app/data
    # ... other configuration

volumes:
  load-tester-data:
    driver: local
```

**Start the application:**

```bash
docker-compose up -d
```

**Data location:** Docker manages the volume at `/var/lib/docker/volumes/load-tester-data`

---

### Using Docker CLI

#### Option 1: Named Volume (Recommended)

```bash
# Create a named volume
docker volume create load-tester-data

# Run container with volume
docker run -d \
  --name pipeops-load-tester \
  -p 8080:8080 \
  -v load-tester-data:/home/pipeops/app/data \
  pipeops-load-tester
```

#### Option 2: Bind Mount (Local Development)

```bash
# Mount a local directory
docker run -d \
  --name pipeops-load-tester \
  -p 8080:8080 \
  -v $(pwd)/data:/home/pipeops/app/data \
  pipeops-load-tester
```

---

## Kubernetes Persistent Volume

### Using PersistentVolumeClaim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: load-tester-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: load-tester
spec:
  replicas: 1
  selector:
    matchLabels:
      app: load-tester
  template:
    metadata:
      labels:
        app: load-tester
    spec:
      containers:
        - name: load-tester
          image: pipeops-load-tester:latest
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: data
              mountPath: /home/pipeops/app/data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: load-tester-pvc
```

**Important:** SQLite is not recommended for multi-replica deployments. Keep replicas: 1.

---

## Volume Management

### Inspect Volume

```bash
# Docker
docker volume inspect load-tester-data

# Kubernetes
kubectl describe pvc load-tester-pvc
```

### Backup Database

```bash
# Docker - Copy from container
docker cp pipeops-load-tester:/home/pipeops/app/data/loadtest.db ./backup-$(date +%Y%m%d).db

# Docker - Copy from volume
docker run --rm \
  -v load-tester-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/loadtest-backup-$(date +%Y%m%d).tar.gz -C /data .

# Kubernetes - Copy from pod
kubectl cp load-tester-pod:/home/pipeops/app/data/loadtest.db ./backup-$(date +%Y%m%d).db
```

### Restore Database

```bash
# Docker - Copy to container
docker cp ./backup.db pipeops-load-tester:/home/pipeops/app/data/loadtest.db

# Docker - Copy to volume
docker run --rm \
  -v load-tester-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "cd /data && tar xzf /backup/loadtest-backup.tar.gz"

# Kubernetes - Copy to pod
kubectl cp ./backup.db load-tester-pod:/home/pipeops/app/data/loadtest.db
```

### Clean Up Volume

```bash
# Docker - Remove volume (DELETES ALL DATA)
docker-compose down -v

# Or manually
docker volume rm load-tester-data

# Kubernetes - Delete PVC (DELETES ALL DATA)
kubectl delete pvc load-tester-pvc
```

---

## Local Development

For local development without Docker:

```bash
# Database is automatically created in ./data directory
go run main.go

# Database files will be at:
# ./data/loadtest.db
# ./data/loadtest.db-shm
# ./data/loadtest.db-wal
```

The application automatically creates the `./data` directory if it doesn't exist.

---

## Database Schema

See `migrations/` directory for database schema and migration history.

Current schema:

- `test_runs` - Test execution metadata and results
- `request_metrics` - Individual request performance metrics

---

## Important Notes

1. **SQLite Limitations**: SQLite is not suitable for multi-node deployments. For horizontal scaling, consider PostgreSQL or MySQL.

2. **File Locks**: Only one application instance should access the database at a time.

3. **WAL Mode**: The application uses WAL mode for better concurrent read performance. All three files (db, shm, wal) must be backed up together.

4. **Permissions**: The `data` directory must be writable by the application user (UID 1000 in Docker).

5. **Storage Size**: Plan for approximately 100MB per 10,000 test runs with full metrics.

---

## Monitoring

Check database size:

```bash
# Docker
docker exec pipeops-load-tester du -sh /home/pipeops/app/data

# Local
du -sh ./data

# Kubernetes
kubectl exec load-tester-pod -- du -sh /home/pipeops/app/data
```

---

## Security

- Database files contain test results and potentially sensitive data
- Ensure volume permissions are restricted
- Consider encrypting volumes in production
- Regularly backup to secure storage

---
