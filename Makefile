CWD=$(shell pwd)
APP_USER=${USER}
CONTAINER_NAME=bob-the-schuleder
BIN_NAME=bob-the-schuleder
RUN_ARGS=-v=true -p=true

default: binary

binary: build clean
	docker run -v $(CWD):/app -it --rm $(CONTAINER_NAME) \
		go build -o $(BIN_NAME)

run: binary
	./$(BIN_NAME) $(RUN_ARGS)

runshell: build
	docker run -v $(CWD):/app -it --rm $(CONTAINER_NAME) /bin/bash

clean:
	rm -f $(BIN_NAME)

upgrade-dependencies: build
	docker run -v $(CWD):/app -it --rm $(CONTAINER_NAME) go get -u

build:
	docker build --build-arg=APP_USER=$(APP_USER) -t $(CONTAINER_NAME) .
