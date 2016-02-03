# Демо для Union FS Docker'а

Представим себе ситуацию. У вас есть боевой сервер MySQL, и объем базы там сотни
гигабайт. При этом для разработки вам хочется иметь свою копию, причём актуальную.

В этой демке я покажу простую схему, как с помощью докера и "слоёным" ФС можно
получать актуальную копию базы данных за секунды.

### Disclaimer

Это демка. В реальных условиях всё будет несколько сложнее.
Многое зависит от выбранной ФС, скажем AUFS покажет себя сильно хуже под
нагрузкой, чем OverlayFS или ZFS. Ну и вообще тут много упрощений.

Не пытайтесь это повторить на боевом :)

## Шаг 0. Запуск "боевого" сервера.

Для начала запустим эмуляцию "боевого" MySQL сервер:

    $ docker run -d \
        --name=production-mysql \
        --net=vb-net \
        -p 3306:3306 \
        -v /var/lib/mysql:/var/lib/mysql \
        -e 'affinity:container!=slave-mysql' \
        -e MYSQL_ROOT_PASSWORD=mysql mysql:5.6 \
        --server-id=42 --log-bin-index=/tmp/mysql-bin-log --log-bin=/var/lib/mysql/bin

Этой командой мы говорим запустить контейнер с именем `production-mysql`
в фоне (`-d`) из образа `mysql:5.6` (https://hub.docker.com/_/mysql/).
Для контейнера мы хотим использовать ранее созданную сеть `vb-net`, порт 3306
из контейнера прокинуть на такой же порт на хостовом интерфейсе `-p 3306:3306`
(не требуется в данном случае, но упрощает отладку). Так же в контейнер мы хотим
примонтировать директорию /var/lib/mysql по такому же пути в контейнере (`-v /var/lib/mysql:/var/lib/mysql`).
Для контейнера мы задаем переменную окружения с паролем для root'а - `-e MYSQL_ROOT_PASSWORD=mysql`
И последний аргумент уходит напрямую в mysqld, запущенный в контейнере (нам нужны
уникальные id сервера для репликации). В ответ `docker run` вернёт нам ID
свежесозданного контейнера. Его стоит сохранить, например в переменную окружения.
Далее в коде она будет называться `$production-mysql-id`

Процесс может занять несколько минут, т.к. докеру может потребоваться скачать этот образ,
если его нет на нужном сервере.

Теперь мы можем подключится к контейнеру или через адрес хоста, на котором он
запущен или через внутренний хост в `vb-net`. Но для этого надо сначала узнать его адрес:

    $ docker inspect \
        --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' \
        $production-mysql-id)

В ответ мы получим IP-адрес, его стоит так же сохранить в переменную, далее он
будет часто использоваться. Назовём её `$masterip`

Зная его адрес, мы можем подключится к нему через консольный клиент:

    $ echo "show variables LIKE 'server_id';" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$masterip -uroot -pmysql

В ответ мы должны увидеть примерно такой вывод:

    Variable_name   Value
    server_id   42

## Шаг 1. Запуск магического MySQL сервера в докере на union fs.

Команда для запуска почти такая же, как и для "боевого", но есть отличия:

    $ docker run -d \
        --name=slave-mysql \
        --net=vb-net \
        -p 3306:3306 \
        -e 'affinity:container!=production-mysql' \
        -e MYSQL_ROOT_PASSWORD=mysql mysql:5.6 \
        --server-id 58 --datadir=/var/lib/slave-mysql

Самое важное отличие тут в том, что мы не монтируем директорию в контейнер и более
того, последним аргументом мы говорим mysqld писать в другую директорию, потому
что нам надо чтобы данные были в контейнере.

Так же для чистоты эксперимента мы хотим, чтобы контейнер был запущен на любом
другом хосте, кроме того, где запушен "боевой" mysql (`-e 'affinity:container!=production-mysql'`).
Ну и передаем другой server-id, чтобы репликация работала.

После запуска можно проверить что всё как мы ожидали:

    $ docker ps --format "table {{.ID}}\t{{.Image}}\t{{.Command}}\t{{.Status}}\t{{.Names}}"

    CONTAINER ID        IMAGE               COMMAND                  STATUS              NAMES
    bb188b2a051c        mysql:5.6           "/entrypoint.sh --ser"   Up 2 minutes        swarm-node-01/slave-mysql
    f88c2ca590ee        mysql:5.6           "/entrypoint.sh --ser"   Up About an hour    swarm-node-02/production-mysql
    e939684b950a        progrium/consul     "/bin/start -server -"   Up About an hour    swarm-node-01/consul

Из таблицы видно, что "боевой" сервер запущен на хосте `swarm-node-02`, а слейв на `swarm-node-01`
По аналогии с мастером IP-адрес слейва сохраняем в $slaveip

## Шаг 2. Теперь нам надо настроить репликацию.

Создаем базу на мастере:

    $ echo "CREATE DATABASE docker;" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$masterip -uroot -pmysql

Даем права на репликацию юзеру `slave_user` с паролем `password`:

    $ echo "GRANT REPLICATION SLAVE ON *.* TO 'slave_user'@'%' IDENTIFIED BY 'password';FLUSH PRIVILEGES;" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$masterip -uroot -pmysql

Проверяем что мастер это мастер:

    $ echo "SHOW MASTER STATUS;" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$masterip -uroot -pmysql

Ожидаем что-то типа такого:

    File    Position    Binlog_Do_DB    Binlog_Ignore_DB    Executed_Gtid_Set
    bin.000002  495

Создаем базу на слейве:

    $ echo "CREATE DATABASE docker;" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$slaveip -uroot -pmysql

Говорим кто у нас мастер:

    $ echo "CHANGE MASTER TO MASTER_HOST='$masterip',\
        MASTER_USER='slave_user', \
        MASTER_PASSWORD='password', \
        MASTER_LOG_FILE='bin.000002', \
        MASTER_LOG_POS=495;" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$slaveip -uroot -pmysql

Где значения `bin.000002` и `495` из `SHOW MASTER STATUS` выполненного ранее.

Запускаем слейв:

    $ echo "START SLAVE;" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$slaveip -uroot -pmysql

Проверяем что всё ок:

    $ echo "SHOW SLAVE STATUS\G" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$slaveip -uroot -pmysql

Ожидаем в начале вот такое сообщение:

    *************************** 1. row ***************************
               Slave_IO_State: Waiting for master to send event


Созданим табличку на мастере, через запись в которую будем тестировать репликацию:

    $ echo "USE docker; CREATE TABLE `repl_status` ( \
      `id` int(11) unsigned NOT NULL AUTO_INCREMENT, \
      `ts` timestamp NOT NULL ON UPDATE CURRENT_TIMESTAMP, \
      PRIMARY KEY (`id`) \
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8;" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$masterip -uroot -pmysql

И на всякий случай проверяем что она появилась на слейве:

    $ echo "USE docker; SHOW CREATE TABLE repl_status\G" | docker run \
        -i --rm --net=vb-net olegfedoseev/mysql-client -h$slaveip -uroot -pmysql

На этом подготовка почти завершена.

# Шаг 3. "Эмулятор" боевой нагрузки.

У нас есть базы, мастер и слейв, но они пустые. Нам нужно чтобы туда кто-то
писал, чтобы данные менялись и было видно жизнь :)

Эмулятором будет простое приложение на Go, которое раз в секунду вставляет строку
в ранее созданную таблицу `repl_status`. Код можно посмотреть в директории `mysql-prod`.
Запускать мы его будем так же в контейнере, для этого нам сначала нужно сделать образ.
Запускаем `docker build --rm -t mysql-prod .` в директории `mysql-prod` и ждём
пока докер сделает свою магию. После чего остается просто запустить его:

    docker run -d --name mysql-prod --net=vb-net -e MYSQL_HOST=$masterip mysql-prod


Теперь у нас есть "боевой" сервер, куда пишет данные наш эмулятор "боевого"
приложения, и к этому боевому серверу настроен слейв, который все изменения
забирает себе.

## Шаг 4. Основная магия с слоями

Основная магия заключается в трех простых действиях:
- Залочить таблицы, чтобы MySQL скинул все данные на диск
- Сохранить изменения ФС контейнера слейва в новый слой и новый образ
- Разблокировать таблицы

После этого можно создавать новый контейнер с базой из этого нового образа.

Для этого сделаем ещё одно маленькое приложение на Go, с использованием
официального API докера.

Исходный код можно посмотреть в директории `snapshooter`.

Теперь всё что нам нужно сделать, это запустить его и на выходе будет
новый образ.