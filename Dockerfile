FROM golang:1.22-bookworm

ARG APP_USER

RUN apt-get update
RUN yes | apt-get upgrade
RUN yes | apt-get install git curl vim

RUN useradd -ms /bin/bash ${APP_USER}

USER ${APP_USER}

ADD .docker/vimrc /home/${APP_USER}/.vimrc
RUN curl -fLo ~/.vim/autoload/plug.vim --create-dirs \
        https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
RUN vim -E -s -u ~/.vimrc +PlugInstall +qall

ENV GO111MODULE=on

WORKDIR /app

