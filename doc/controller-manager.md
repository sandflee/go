# controller manager
## replication controller
作用：负责维护pod数目和rc期望的pod数目一致
实现：
监听apiserver中的rc和pod,当pod发生变化时，找到相应的rc,做同步，rc变化时同样做同步。
同步过程：
- 比较rc中期望的pod数目，如果跟podCache中的数目不同，调用apiserver接口增加／删除pod
- 调用apiserver接口把rc.status.replica字段更新为最新的

## garbage-collector
每隔２０s，将系统中已经结束的pod（pod.status.phase not in (RUNNING,PENDING,UNKNOWN)）从apiserver删除

## node-controller
node ready -> 非ready
将node上所有pod的readyConditioin设置为false
如果持续很长时间处于非ready状态，将node上的pod清理交给其他routine清理,根据 pod.DeletionGracePeriodSeconds的设置又分为实时清理和延迟清理。
有一个routine定期扫面绑定在node上的pod (pod.spec.nodemame != ""),  如果对应的node在nodeCache中找不到了，删除这个pod

## service-controller
维护service和loadBlancher的对应关系
- service有变化，　创建/删除对应的lb
- node发生变化，　调用lb update接口更新hosts


## endpoint-controller
维护service和endPoint对象的映射关系
- 启动时获取所有endPoint对象，同步对应的service
- watch service, 如果有service发生变化，同步service
- 同步service罗辑：　
-- 如果serivce已经删除，删除对应的endpoint对象
-- 如果service增加／变化，获取所有相关pod信息，利用podId,生成endPoint对象，调用apiserver接口进行更新
- 例子
-- service : {"ports":[{"protocol":"TCP","port":8000,"targetPort":80}],"clusterIP\":\"10.0.0.72\"}
-- endpoint : {"addresses":[{"ip":"1.1.1.1"},{"ip":"1.1.1.2"}],"ports":[{"port":80,"protocol":"TCP"}]}
- endpoint对象的变化由kube-proxy捕获，维护对应的路由信息


## resourcequota-controller

## namespace-controller
namespace 创建后处于active状态，可以在namespace下进行操作
如果删除namespace, 处于terminating状态，Namespace.ObjecMeta.DeletionTimestamp被设置为当前时间，namespace controller发现这一事件，清理namespace下已知的资源，清理完成后将"kubernetes"从Namespace.Spec.Finalizers中删除
Namespace.Spec.Finalizers为空时，把namespace从etcd中删除

## horizontal-pod-autoscaler

## daemon-set-controller

## job-controller

## deployment-controller

## replicasets

## persistent-volume-binder

## persistent-volume-recycler

## persistent-volume-provisioner

## tokens-controller

## service-account-controller