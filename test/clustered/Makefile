.PHONY: help test-3node-write-heavy test-3node-read-heavy test-3node-mixed \
        test-6node-write-heavy test-6node-read-heavy test-6node-mixed

help:
	@echo "Valkey Cluster Scalability Tests"
	@echo ""
	@echo "Available targets:"
	@echo ""
	@echo "3-Node Cluster Tests (0.45 CPU total):"
	@echo "  test-3node-write-heavy    - Run write-heavy test on 3-node cluster"
	@echo "  test-3node-read-heavy     - Run read-heavy test on 3-node cluster"
	@echo "  test-3node-mixed          - Run mixed test on 3-node cluster"
	@echo ""
	@echo "6-Node Cluster Tests (0.90 CPU total):"
	@echo "  test-6node-write-heavy    - Run write-heavy test on 6-node cluster"
	@echo "  test-6node-read-heavy     - Run read-heavy test on 6-node cluster"
	@echo "  test-6node-mixed          - Run mixed test on 6-node cluster"
	@echo ""

# 3-Node Cluster Tests
test-3node-write-heavy:
	@./run-cluster-3-nodes.sh write-heavy

test-3node-read-heavy:
	@./run-cluster-3-nodes.sh read-heavy

test-3node-mixed:
	@./run-cluster-3-nodes.sh mixed

# 6-Node Cluster Tests
test-6node-write-heavy:
	@./run-cluster-6-nodes.sh write-heavy

test-6node-read-heavy:
	@./run-cluster-6-nodes.sh read-heavy

test-6node-mixed:
	@./run-cluster-6-nodes.sh mixed
