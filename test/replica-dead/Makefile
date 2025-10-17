.PHONY: up down test status clean

up:
	docker-compose up -d

test:
	docker-compose up benchmark

down:
	docker-compose down

clean:
	docker-compose down -v

status:
	@docker exec valkey-primary valkey-cli INFO replication | grep -E "role|connected_slaves"
	@docker exec valkey-replica valkey-cli INFO replication | grep -E "role|master_link_status"

logs:
	docker logs -f bridge
