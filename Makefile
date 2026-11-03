# Shared command interface. Stubs fail until implemented, and run inside WSL2.
# Destructive resets must stay separate commands with clear names.
.PHONY: help dev dev-local stop-dev deploy-lab stop-lab start-lab backup-lab restore-test test 

help:
	@echo "  dev            Run locally against private service host dependencies; fail helpfully if unreachable"
	@echo "  dev-local      Run self-contained with separate disposable data"
	@echo "  stop-dev       Stop this project without deleting data"
	@echo "  deploy-lab     Deploy a committed version, migrate once, run smoke tests"
	@echo "  stop-lab       Stop this deployed app only"
	@echo "  start-lab      Start this deployed app only"
	@echo "  backup-lab     Make a consistent backup"
	@echo "  restore-test   Restore the latest backup in isolation"
	@echo "  test           Run this project checks"

# Run locally against private service host dependencies; fail helpfully if unreachable
dev:
	@echo "dev is not implemented yet" >&2; exit 1

# Run self-contained with separate disposable data
dev-local:
	@echo "dev-local is not implemented yet" >&2; exit 1

# Stop this project without deleting data
stop-dev:
	@echo "stop-dev is not implemented yet" >&2; exit 1

# Deploy a committed version, migrate once, run smoke tests
deploy-lab:
	@echo "deploy-lab is not implemented yet" >&2; exit 1

# Stop this deployed app only
stop-lab:
	@echo "stop-lab is not implemented yet" >&2; exit 1

# Start this deployed app only
start-lab:
	@echo "start-lab is not implemented yet" >&2; exit 1

# Make a consistent backup
backup-lab:
	@echo "backup-lab is not implemented yet" >&2; exit 1

# Restore the latest backup in isolation
restore-test:
	@echo "restore-test is not implemented yet" >&2; exit 1

# Run this project checks
test:
	@echo "test is not implemented yet" >&2; exit 1

