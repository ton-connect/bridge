#!/bin/bash
# Verification script for Valkey sharding setup

echo "=== Verifying Data Distribution ==="

# Check cluster status
echo "Cluster Info:"
docker exec bridge3-valkey-shard1-1 valkey-cli -c cluster info

echo -e "\nCluster Nodes:"
docker exec bridge3-valkey-shard1-1 valkey-cli -c cluster nodes

# Check key distribution
echo -e "\n=== Key Distribution ==="
echo "Shard 1 keys:"
docker exec bridge3-valkey-shard1-1 valkey-cli dbsize

echo "Shard 2 keys:"
docker exec bridge3-valkey-shard2-1 valkey-cli -p 6380 dbsize

echo "Shard 3 keys:"
docker exec bridge3-valkey-shard3-1 valkey-cli -p 6381 dbsize

# Sample some keys to see which shard they're on
echo -e "\n=== Sample Key Locations ==="
for i in {1..10}; do
  key="test:key:$i"
  slot=$(docker exec bridge3-valkey-shard1-1 valkey-cli -c cluster keyslot $key)
  node=$(docker exec bridge3-valkey-shard1-1 valkey-cli -c cluster nodes | grep $slot | head -1)
  echo "Key '$key' -> Slot $slot"
done

# Check memory usage on each shard
echo -e "\n=== Memory Usage ==="
echo "Shard 1 memory:"
docker exec bridge3-valkey-shard1-1 valkey-cli info memory | grep used_memory_human

echo "Shard 2 memory:"
docker exec bridge3-valkey-shard2-1 valkey-cli -p 6380 info memory | grep used_memory_human

echo "Shard 3 memory:"
docker exec bridge3-valkey-shard3-1 valkey-cli -p 6381 info memory | grep used_memory_human

echo -e "\n=== Hash Slot Distribution ==="
echo "Shard 1 slots:"
docker exec bridge3-valkey-shard1-1 valkey-cli -c cluster nodes | grep "172.20.0.20:6379" | awk '{print $9}'

echo "Shard 2 slots:"
docker exec bridge3-valkey-shard1-1 valkey-cli -c cluster nodes | grep "172.20.0.21:6380" | awk '{print $9}'

echo "Shard 3 slots:"
docker exec bridge3-valkey-shard1-1 valkey-cli -c cluster nodes | grep "172.20.0.22:6381" | awk '{print $9}'
