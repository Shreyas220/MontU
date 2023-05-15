.PHONY: build-image
build-image:
	docker build . -t gladium08/kiem:json
	docker push gladium08/kiem:json

