.PHONY: gen gen-check test-gen

gen:
	cd server && go generate ./...
	cd web && npm run gen

gen-check:
	python3 api/check_generated.py

test-gen:
	python3 -B -m unittest discover -s api
	python3 api/check_generated.py --test-drift
	python3 -B -m unittest discover -s clients/python/tests
