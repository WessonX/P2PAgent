## What is the library used for?

该代码库为wynhowxie的毕业设计而生。基本功能是前端页面通过部署在本机和机器人上的go程序，实现p2p网络连接，进而与机器人上的ros_server通信，从而对机器人进行网络控制。

## Directory  structure of the program

### Agent

这个文件夹主要存放的是agent.go。作为Agent对象（在go里我们称为structure结构体），声明定义了相关的属性、方法。该类解释了端对端通信的过程方法。

该文件夹还存放了control_unix.go和control_windows.go。这两个文件主要定义了control方法，该方法用于设定socket的端口复用，分别针对的是unix（类unix，like macOS），windows平台。

### LocalAgent

这个文件夹存放的是LocalAgent.go程序，以及localAgent可执行文件。**注意这里的localAgent是在mac平台下编译的。如果使用其他平台，请自行交叉编译。**

localAgent运行在本机，或者说，客户端。在通信过程中，前端页面会和localAgent建立websocket连接，然后localAgent会和运行在机器人上的rosAgent进行p2p通信，最后rosAgent会和机器人上的ros_server建立连接。localAgent是对前述的Agent对象的具体应用。简而言之，它是客户端的网络代理。

### RosAgent

这个文件夹存放的是RosAgent.go程序，以及rosAgent可执行文件。**注意这里的rosAgent是在mac平台下交叉编译成linux平台下使用的。如果使用其他平台，请自行交叉编译.**

和localAgent类似，rosAgent运行在机器人端。rosAgent负责与localAgent建立点对点通信，并和ros_server建立websocket连接，在localAgent与ros_server之间进行信息交换。rosAgent也是对Agent对象的具体应用。它是机器人端的网络代理。

## Server

这个文件夹存放的是server.go以及server可执行文件。**注意这里的server是在mac平台下交叉编译成linux平台下使用的。如果使用其他平台，请自行交叉编译**

server主要负责协助两个peer节点（即localAgent和rosAgent）建立p2p连接。localAgent和rosAgent启动后就向server发送信息，将自己的私网地址发送出去，server会给主动连接进来的节点分配一个uuid，记录下它们的公网地址，然后将uuid和公网地址一并返回给peer节点。

此时每一个和server建立了连接的节点，就知道了自己在整个通信网络中的公网地址，以及uuid

当localAgent向server查询目标uuid的时候，server就将对应节点（rosAgent）的公网地址和私网地址发送给local Agent。同时也将localAgent节点的对应信息发送给rosAgent。

此时双端节点就同时拥有了自己和对方的公网地址、局域网地址。

如果双方的公网地址一样，说明二者位于同一个局域网下，则互相给对方的局域网地址发送消息，建立局域网连接；

如果不一样，则互相给对方的公网地址发送消息，尝试内网穿透（如果双方路由都支持ipv6，则会进行ipv6直连，而不叫内网穿透）

### Test

该文件夹不重要，主要是进行一些代码测试.

### Utils

该文件夹存放了utils.go。主要存放一些工具方法。

## About p2p between localAgent and RosAgent

在localAgent与rosAgent之间进行连接时，主要有三种策略：

+ 先尝试ipv6连接。若双方路由都支持ipv6，则ipv6直连。

+ 若ipv6连接尝试失败。则判断对方的公网地址和自己的是不是一样。如果一样，说明“可能”是处于同一个局域网下，则尝试直连对方的局域网地址。

  note：这里说可能，是因为可能存在多重nat，所以就算两个peer的公网地址相同，它们可能实际处于一个大的局域网下的两个不同的小局域网。

+ 如果不一样，或者局域网连接失败，则尝试tcp打洞穿透。

+ 如果打洞也失败，则返回一个错误信息给前端页面，前端页面会改去连接公网服务器的指定端口，通过frp的方案与ros_server建立连接。

## How to use

+ 获取代码仓库

  ```
  git clone https://github.com/WessonX/P2PAgent.git
  ```

+ 在公网服务器上打开3001端口，并运行server程序。运行环境为linux
+ 在客户端打开3001端口，并运行localAgent程序。注意在代码的177行和182行配置公网服务器的ipv6地址和ipv4地址。运行环境为macOS

+ 在机器人端打开3002端口，并运行serverAgent程序。注意在代码的132行和137行配置公网服务器的ipv6地址和ipv4地址。运行环境为linux

+ 在客户端，输入localhost：3001，前端即可连接上ros_server，并互传信息。

