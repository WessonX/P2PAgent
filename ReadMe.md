## What is the library used for?

基本功能是前端页面通过部署在本机和机器人上的go程序，实现p2p网络连接，进而与机器人上的ros_server通信，从而对机器人进行网络控制。

## Directory  structure of the program

### Agent

agent.go: 作为Agent对象（在go里我们称为structure结构体），声明定义了相关的属性、方法。该类解释了端对端通信的过程方法。

control_unix.go和control_windows.go: 这两个文件主要定义了control方法，该方法用于设定socket的端口复用，分别针对的是unix（类unix，like macOS），windows平台。

### Common
common.go: 定义了中继服务器的地址

### LocalAgent

localAgent.go: 源代码
localAgent: 可执行文件。“localAgent”为arm linux，localAgent.exe 为amd64 windows,**其他平台请自行编译**

localAgent运行在本机，或者说，客户端。在通信过程中，前端页面会和localAgent建立websocket连接，然后localAgent会和运行在机器人上的rosAgent进行p2p通信，最后rosAgent会和机器人上的ros_server建立连接。localAgent是对前述的Agent对象的具体应用。简而言之，它是客户端的网络代理。

localagent.reg: 注册表，用在windows平台注册自定义的url，以便能够在前端页面直接唤起localAgent。

### RosAgent

rosAgent.go: 源代码
rosAgent： 可执行文件。**注意rosAgent是arm linux编译产物。如果使用其他平台，请自行交叉编译.**

和localAgent类似，rosAgent运行在机器人端。rosAgent负责与localAgent建立点对点通信，并和ros_server建立websocket连接，在localAgent与ros_server之间进行信息交换。rosAgent也是对Agent对象的具体应用。它是机器人端的网络代理。

frpc.service: 用于在机器人端实现frp的自启。

## Server

server.go: 源代码 
server: 可执行文件。**注意这里的server是arm linux编译产物。如果使用其他平台，请自行交叉编译**
frps.service: 用于frps的自启
relayServer.service: 用于上面的server可执行文件的自启

server主要负责协助两个peer节点（即localAgent和rosAgent）建立p2p连接。localAgent和rosAgent启动后就向server发送信息，将自己的私网地址和ipv6地址（如果没有就为空）发送出去，server会给主动连接进来的节点分配一个uuid，记录下它们的公网地址，然后将uuid和公网地址一并返回给peer节点。

此时每一个和server建立了连接的节点，就知道了自己在整个通信网络中的公网地址，以及uuid

当localAgent向server查询目标uuid的时候，server就将对应节点（rosAgent）的公网地址，ipv6地址和私网地址发送给local Agent。同时也将localAgent节点的对应信息发送给rosAgent。

此时双端节点就同时拥有了自己和对方的公网地址、局域网地址以及ipv6地址。

如果双方的公网地址一样，说明二者位于同一个局域网下，则互相给对方的局域网地址发送消息，建立局域网连接；

如果不一样，则互相给对方的公网地址发送消息，尝试内网穿透（如果双方路由都支持ipv6，则会进行ipv6直连，而不叫内网穿透）

frps.service: 用于实现在机器人上的frp自启

relayServer.service: 用于实现在机器人上，中继程序的自启

### Test

该文件夹不重要，主要是进行一些代码测试.

### Utils

该文件夹存放了utils.go。主要存放一些工具方法。

### Common
该文件夹下的common.go，存放常量，如relay_addr

### frp
该文件夹包含了frps、frpc的可执行文件和配置文件。
frpc: 运行在机器人上， 开机自启的service为RosAgent/frpc.service
frpc.ini: frpc的配置文件，需要配置中继服务器的地址，以及token
frps: 运行在中继服务器上，开机自启的service为server/frps.service
frps.ini: frps的配置文件，需要配置token

**note: 这里的fprc,fprs,对应的架构为linux arm-64，如果是其他的系统架构，请自行到frp官网去下载**

## About p2p between localAgent and RosAgent

在localAgent与rosAgent之间进行连接时，主要有三种策略：

+ 先尝试局域网直连。

+ 若局域网直连失败，尝试ipv6连接。若双方路由都支持ipv6，则ipv6直连。
+ 如果ipv6连接失败，则尝试tcp打洞穿透。
+ 如果打洞也失败，则返回一个错误信息给前端页面，前端页面会改去连接公网服务器的指定端口，通过frp的方案与ros_server建立连接。

## How to use

### 获取代码仓库到前端、机器人端、服务器端

```
git clone https://github.com/WessonX/P2PAgent.git
```

### 服务器端

+ 在公网服务器上打开3001端口，并运行server程序。运行环境为linux arm。
+ 将frps.service拷贝到/etc/systemd/system目录，类比机器人端的代码,实现frp的开机自启
+ 将relayServer.service拷贝到/etc/systemd/system目录，类比机器人端的代码,实现frp的开机自启

### 前端（i.e.客户端）

+ 在客户端打开3000，3003端口，并运行localAgent程序。注意在common.go中配置中继服务器的ipv4地址。
+ 启动rosUI

### 机器人端

+ 将frpc.service拷贝到/etc/systemd/system目录，执行以下代码,实现frp的开机自启

  ```
  sudo systemctl dameon-load
  sudo systemctl start frpc.service
  sudo systemctl status frpc.service
  // 如果ok，继续往下执行
  sudo systemctl enable frpc.service
  ```

+ 进入arebot_ws目录，编译好后，roslaunch arebot_bringup arebot.launch
+ 在机器人端打开3002端口，并运行rosAgent程序。



以上各端都执行好后，将rosAgent输出的uuid，输入到前端的连接页面，即可连接成功
