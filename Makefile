CWD=$(shell pwd)
APP_USER=${USER}
CONTAINER_NAME=bob-the-scheduler
TEST_CONTAINER_NAME=bob-the-scheduler
BIN_NAME=bob-the-scheduler
RUN_ARGS=-v -o=$(shell pwd)/logs

default: binary

binary: build clean
	docker run -v $(CWD):/app -it --rm $(CONTAINER_NAME) \
		go build -o $(BIN_NAME)

run: binary
	./$(BIN_NAME) $(RUN_ARGS)

run-docker: binary
	docker build -t $(TEST_CONTAINER_NAME) -f Dockerfile.test .
	docker run -p 8000:8000 -it --rm $(TEST_CONTAINER_NAME)

runshell: build
	docker run -v $(CWD):/app -it --rm $(CONTAINER_NAME) /bin/bash

clean:
	rm -f $(BIN_NAME)

upgrade-dependencies: build
	docker run -v $(CWD):/app -it --rm $(CONTAINER_NAME) go get -u

build:
	docker build --build-arg=APP_USER=$(APP_USER) -t $(CONTAINER_NAME) .
