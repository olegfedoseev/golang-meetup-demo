# Создание кластера swarm

Swarm может использовать несколько механизмов хранения списка хостов в кластере,
но самый распространённый и популярный - consul

Его и будем использовать. Для начала создадим отдельную машину под consul:

    $ docker-machine create -d virtualbox --virtualbox-disk-size="2000" consul-master
    Creating VirtualBox VM...
    Creating SSH key...
    Starting VirtualBox VM...
    Starting VM...
    To see how to connect Docker to this machine, run: docker-machine env consul-master

`docker-machine` принимает много аргументов, в данном случае мы говорим
использовать драйвер для VirtualBox'а (`-d virtualbox`), и создать машину с
диском в 2Гб, вместо дефолтных 20Гб (`--virtualbox-disk-size="2000"`) и
говорим что называться она будем `consul-master`


По аналогии можно создавать машины в AWS, Azure, DigitalOcean, Rackspace и т.д.


Когда у нас запущена машина для consul'а, настало время запустить сам consul:

    docker $(docker-machine config mh-keystore) run \
        -d -p "8500:8500" --name="consul" --hostname "consul" \
        progrium/consul -server -bootstrap


Теперь нам надо сделать машину с swarm-master, это может быть отдельная машина,
или одна из кластера:

    docker-machine create -d virtualbox \
        --swarm \
        --swarm-master \
        --swarm-discovery="consul://$(docker-machine ip consul-master):8500" \
        --engine-opt="cluster-store=consul://$(docker-machine ip consul-master):8500" \
        --engine-opt="cluster-advertise=eth1:2376" \
        swarm-master

Тут уже команда по сложнее. Мы так же говорим что хотим VirtualBox.
И указываем что нам нужна машина с swarm (`--swarm`), причём мастер (`--swarm-master`),
а для дискавери мы хотим использовать consul по указанному хосту, IP-адрес мы
получаем через специальную команду docker-machine
(`--swarm-discovery="consul://$(docker-machine ip consul-master):8500"`).
Далее мы указываем две опции для docker engine. Говорим искать данные о кластере
в том же consul'е - (`--engine-opt="cluster-store=consul://$(docker-machine ip consul-master):8500"`)
и указываем какой интерфейс/ip-адрес использовать в качестве для доступа к этой
ноде из кластера. Это может быть как внутренний адрес, так и внешний, главное
чтобы у остальных нод в кластере был до него доступ.

После чего остается создать нужно кол-во нод для остального кластера:

    docker-machine create -d virtualbox \
        --swarm \
        --swarm-discovery="consul://$(docker-machine ip consul-master):8500" \
        --engine-opt="cluster-store=consul://$(docker-machine ip consul-master):8500" \
        --engine-opt="cluster-advertise=eth1:2376" \
        swarm-node-01

    docker-machine create -d virtualbox \
        --swarm \
        --swarm-discovery="consul://$(docker-machine ip consul-master):8500" \
        --engine-opt="cluster-store=consul://$(docker-machine ip consul-master):8500" \
        --engine-opt="cluster-advertise=eth1:2376" \
        swarm-node-02

Тут всё тоже самое, за исключением имени, и отсутствия `--swarm-master`.

В завершение создадим [общую сеть](https://docs.docker.com/engine/userguide/networking/get-started-overlay/):

    docker network create --driver overlay vb-net

Эта команда говорит докеру создать новую сеть, с `overlay` драйвером с именем `vb-net`

Теперь можно всё это проверить:

    docker $(docker-machine config --swarm swarm-master) info

С помощью `$(docker-machine config --swarm swarm-master)` мы указываем клиенту докера
ходить не в swarm manager.

В ответ будет что-то типа такого:

    Containers: 7
    Images: 13
    Role: primary
    Strategy: spread
    Filters: health, port, dependency, affinity, constraint
    Nodes: 2
     swarm-node-01: 192.168.99.103:2376
      └ Status: Healthy
      └ Containers: 5
      └ Reserved CPUs: 0 / 1
      └ Reserved Memory: 0 B / 1.021 GiB
      └ Labels: executiondriver=native-0.2, kernelversion=4.1.17-boot2docker, operatingsystem=Boot2Docker 1.10.0-rc2 (TCL 6.4.1); master : b1ddf2c - Mon Feb  1 17:31:43 UTC 2016, provider=virtualbox, storagedriver=aufs
     swarm-node-02: 192.168.99.104:2376
      └ Status: Healthy
      └ Containers: 2
      └ Reserved CPUs: 0 / 1
      └ Reserved Memory: 0 B / 1.021 GiB
      └ Labels: executiondriver=native-0.2, kernelversion=4.1.17-boot2docker, operatingsystem=Boot2Docker 1.10.0-rc2 (TCL 6.4.1); master : b1ddf2c - Mon Feb  1 17:31:43 UTC 2016, provider=virtualbox, storagedriver=aufs
    CPUs: 2
    Total Memory: 2.043 GiB
    Name: swarm-master
