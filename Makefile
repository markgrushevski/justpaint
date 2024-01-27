# client dev
c:
	cd client && npm run dev
# server dev
s:
	cd server && npm run dev
# lint
l:
	cd server && npm run lint:all && cd ../client && npm run lint:all
# shortcuts for run docker containers
run:
	docker-compose up --build -d frontend api nginx
run-dev:
	docker-compose up --build -d frontend_dev api_dev nginx_dev
stop:
	docker-compose stop frontend api nginx
stop-dev:
	docker-compose stop frontend_dev api_dev nginx_dev
logs:
	docker-compose logs -f
restart:
	make stop && make run
restart-dev:
	make stop-dev && make run-dev
