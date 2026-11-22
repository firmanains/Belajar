.PHONY: up down verify
up: ## nyalakan Postgres (55432) dan Kafka (59092)
	docker compose up -d --wait
down: ## matikan dan hapus datanya
	docker compose down -v
verify: ## cek semua lab: starter gagal sesuai rancangan, solusi lulus
	@for d in labs/*/ proyek/; do [ -d "$$d" ] && ./verify.sh "$$d" || exit 1; done
