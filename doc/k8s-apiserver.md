# add apigroup resource to apiserver

apiserver提供了etcd存储和k8s rst接口的转换.主要包括下面几层：
1. REST接口层，　对用户暴漏RST接口
2. resource storage层，每个RST接口都跟一个storage关联，提供了Create/Update/Delete/Get/Watch等接口。一般基于generic#store实现，每种resource只需要实线特定的stragy,generic#store负责回调
3. cacher层　generic#store的底层存储实现，根据配置分为有cache或者直接读apiserver,具体实线在cacher.go
4. 具体存储层 跟etcd打交道，有etcd集群信息和版本信息，把数据直接更新到etcd，etcd2的实线在storage/etcd_helper.go, etcd3的实线在storage／etcd3/store.go


1, group 和  resource的结构描述
2, resource如何保存到etcd, 涉及到的各种策略
3, 如何把group和resource注册到apiserver，以及跟２结合跟etcd产生关联


## group和resource描述相关
改变api时严格按照官方文档操作　https://github.com/kubernetes/kubernetes/blob/master/docs/devel/api_changes.md
涉及到的文件
- types.go  resource的go struct描述
- types.generated.go  go对象和json之间的相互转换，基于性能的考虑没有用go默认实现,  hack/update-codecgen.sh脚本负责自动生成这个文件
- defaults.go  设置resource字段的默认值
- zz_generated_conversion.go  group对象和version对象自动转换　　./hack/update-codegen.sh 自动生成
- conversion.go 　可以hook group对象和version对象转换
- zz_generated_deepcopy.go  相同对象deepcopy的实现　　./hack/update-codegen.sh 自动生成
- generated.proto  generated.pb.go    对象的pb描述以及go实现 　hack/update-generated-protobuf.sh

- register.go 
KUBE_API_VERSIONS可以指定apiserver启动哪些group
定义了GroupVersion,并提供了一些函数供install.go使用
   
- install.go     
1. 把groupVersion信息向APIRegistrationManager注册，写到registerVersion和enableVersion
2. 把groupversion信息向schme注册
3. 新建GroupMeta信息，并向APIRegistrationManager注册,写到GroupMetaMap
4. 注册RESTMapper

RESTMapper, GroupVersion的作用？

```go
	addVersionsToScheme(externalVersions...)
	preferredExternalVersion := externalVersions[0]

	groupMeta := apimachinery.GroupMeta{
		GroupVersion:  preferredExternalVersion,
		GroupVersions: externalVersions,
		RESTMapper:    newRESTMapper(externalVersions),
		SelfLinker:    runtime.SelfLinker(accessor),
		InterfacesFor: interfacesFor,
	}

	if err := registered.RegisterGroup(groupMeta); err != nil {
		return err
	}
	api.RegisterRESTMapper(groupMeta.RESTMapper)
```


## resource如何存储在etcd
- 每种资源必须生成storage对象，放到ApiGroupInfo#VersionedResourcesStorageMap中，由apiserver负责把storage相应方法注册成rest接口（api_installer.go#registerResourceHandlers）
- 每个资源的实现有大量重复罗辑，所以一般借助generic#registry#store.go实现，创建store时只需要实现相应的func即可，比如如何创建一个对象，更新对象时的策略
- etcd.go 负责生成store对象，　strategy.go 更新和创建对象时的一些策略实现


## resource如何注册到apiserver
每个ApiGroup需要创建ApiGroupInfo,里面包含group的版本，以及每个版本的resource map. ApiServer根据ApiGroup Info将其跟rest接口绑定
#### 1. 创建ApiGroupInfo
groupMeta为前面install.go中注册的GroupMeta, Scheme,ParameterCodec,NegotiatedSerializer为全局变量，不需要额外创建
VersionedResourcesStorageMap　记录了每个GroupVersion都要哪些resource,以及对应的storage实现

```go
genericApiserver.go

func NewDefaultAPIGroupInfo(group string) APIGroupInfo {
	groupMeta := registered.GroupOrDie(group)

	return APIGroupInfo{
		GroupMeta:                    *groupMeta,
		VersionedResourcesStorageMap: map[string]map[string]rest.Storage{},
		OptionsExternalVersion:       &registered.GroupOrDie(api.GroupName).GroupVersion,
		Scheme:                       api.Scheme,
		ParameterCodec:               api.ParameterCodec,
		NegotiatedSerializer:         api.Codecs,
	}
}

APIGroupInfo {
   GroupMeta apimachinery.GroupMeta
   VersionedResourcesStorageMap [][]rest.Storage
   IsLegacyGroup bool
   OptionsExternalVersion *unversioned.GroupVersion

   Scheme *runtime.Scheme
   NegotiatedSerializer runtime.NegotiatedSerializer
   ParameterCodec runtime.ParameterCodec

   SubresourceGroupVersionKind []unversioned.GroupVersionKind
}
```
#### 2. 从ApiGroupInfo生成ApiGroupVersion，对应每个版本的信息
apiGroupInfo对应的是一个ApiGroup的信息，ApiGroupVersion对应其中一个特定版本
```go
func (s *GenericAPIServer) newAPIGroupVersion(apiGroupInfo *APIGroupInfo, groupVersion unversioned.GroupVersion) (*apiserver.APIGroupVersion, error) {
	return &apiserver.APIGroupVersion{
		RequestInfoResolver: s.NewRequestInfoResolver(),

		GroupVersion: groupVersion,

		ParameterCodec: apiGroupInfo.ParameterCodec,
		Serializer:     apiGroupInfo.NegotiatedSerializer,
		Creater:        apiGroupInfo.Scheme,
		Convertor:      apiGroupInfo.Scheme,
		Copier:         apiGroupInfo.Scheme,
		Typer:          apiGroupInfo.Scheme,
		SubresourceGroupVersionKind: apiGroupInfo.SubresourceGroupVersionKind,
		Linker: apiGroupInfo.GroupMeta.SelfLinker,
		Mapper: apiGroupInfo.GroupMeta.RESTMapper,

		Admit:             s.AdmissionControl,
		Context:           s.RequestContextMapper,
		MinRequestTimeout: s.MinRequestTimeout,
	}, nil
}
```

#### 3. ApiGroupVersion中的每个资源注册到apiserver
api_installer.go#registerResourceHandlers把下面storage中的resource和storage做绑定
```go
		tappStorage, tappStatusStorage := tappetcd.NewREST(restOptionsGetter(gaia.Resource("tapps")))
		storage["tapps"] = tappStorage
```


## get/create/update　resource处理逻辑

## generic store实现
### storage　接口

底层storage实现为cacher

- create   BeforeCreate,AfterCreate有很多策略实线
```go
	if err := rest.BeforeCreate(e.CreateStrategy, ctx, obj); err != nil {
		return nil, err
	}
	name, err := e.ObjectNameFunc(obj)
	if err != nil {
		return nil, err
	}
	key, err := e.KeyFunc(ctx, name)
	if err != nil {
		return nil, err
	}
	ttl, err := e.calculateTTL(obj, 0, false)
	if err != nil {
		return nil, err
	}
	out := e.NewFunc()
    e.Storage.Create(ctx, key, obj, out, ttl);
    if e.AfterCreate != nil {
		if err := e.AfterCreate(out); err != nil {
			return nil, err
		}
	}
	if e.Decorator != nil {
		if err := e.Decorator(obj); err != nil {
			return nil, err
		}
	}
```
- update，跟create类似有BeforeUpdate,AfterUpdate等，并且很多处理resourceVersion的逻辑
- delete (todo)
- get 直接从storage取出obj

### rest　策略层
提供了BeforeUpdate/BeforeCreate/BeforeDelete的实现，会回调每个资源的一些创建，更新策略
```go
type RESTCreateStrategy interface {
	runtime.ObjectTyper
	// The name generate is used when the standard GenerateName field is set.
	// The NameGenerator will be invoked prior to validation.
	api.NameGenerator

	// NamespaceScoped returns true if the object must be within a namespace.
	NamespaceScoped() bool
	// PrepareForCreate is invoked on create before validation to normalize
	// the object.  For example: remove fields that are not to be persisted,
	// sort order-insensitive list fields, etc.  This should not remove fields
	// whose presence would be considered a validation error.
	PrepareForCreate(ctx api.Context, obj runtime.Object)
	// Validate is invoked after default fields in the object have been filled in before
	// the object is persisted.  This method should not mutate the object.
	Validate(ctx api.Context, obj runtime.Object) field.ErrorList
	// Canonicalize is invoked after validation has succeeded but before the
	// object has been persisted.  This method may mutate the object.
	Canonicalize(obj runtime.Object)
}

type RESTGracefulDeleteStrategy interface {
	// CheckGracefulDelete should return true if the object can be gracefully deleted and set
	// any default values on the DeleteOptions.
	CheckGracefulDelete(ctx api.Context, obj runtime.Object, options *api.DeleteOptions) bool
}

type RESTUpdateStrategy interface {
	runtime.ObjectTyper
	// NamespaceScoped returns true if the object must be within a namespace.
	NamespaceScoped() bool
	// AllowCreateOnUpdate returns true if the object can be created by a PUT.
	AllowCreateOnUpdate() bool
	// PrepareForUpdate is invoked on update before validation to normalize
	// the object.  For example: remove fields that are not to be persisted,
	// sort order-insensitive list fields, etc.  This should not remove fields
	// whose presence would be considered a validation error.
	PrepareForUpdate(ctx api.Context, obj, old runtime.Object)
	// ValidateUpdate is invoked after default fields in the object have been
	// filled in before the object is persisted.  This method should not mutate
	// the object.
	ValidateUpdate(ctx api.Context, obj, old runtime.Object) field.ErrorList
	// Canonicalize is invoked after validation has succeeded but before the
	// object has been persisted.  This method may mutate the object.
	Canonicalize(obj runtime.Object)
	// AllowUnconditionalUpdate returns true if the object can be updated
	// unconditionally (irrespective of the latest resource version), when
	// there is no resource version specified in the object.
	AllowUnconditionalUpdate() bool
}
```


## cache storage实现
### 创建storage
- master.go 创建generic.RESTOptions时，通过storageDecorator赋值给Decorator, 
```go
master.go
generic.RESTOptions{
		StorageConfig:           storageConfig,
		Decorator:               m.StorageDecorator(),
		DeleteCollectionWorkers: m.deleteCollectionWorkers,
		ResourcePrefix:          c.StorageFactory.ResourcePrefix(resource),
	}
```
```go
genericapiserver.go
func (s *GenericAPIServer) StorageDecorator() generic.StorageDecorator {
	if s.enableWatchCache {
		return registry.StorageWithCacher
	}
	return generic.UndecoratedStorage
}
```
- 每种资源都需要创建store对象，调用RestOption生成具体的storage,执行具体的create/update/get操作
```go
    registry#tapp/etcd#etcd.go
	storageInterface, _ := opts.Decorator(
		opts.StorageConfig,
		cachesize.GetWatchCacheSizeByResource(cachesize.TApp),
		&gaiaapi.TApp{},
		prefix,
		tapp.Strategy,
		newListFunc,
		storage.NoTriggerPublisher,
	)
    store := &registry.Store{
      Storage: storageInterface,
    }
```

### cache storage实现　　todo
1. 会创建一个cacher对象实线storage接口，每个resource都有自己的cacher,
1. 只有watch会走cache, get操作直接从storage取数据？

  
  
RestOptions
``` go
type RESTOptions struct {
     // etcd相关配置，etcd2/etcd3? etcd location/prefix等，还包括codec, resource memory version和storageVersion相互转换
	StorageConfig           *storagebackend.Config
    // storage的修饰器，返回一个func，生成具体的storage接口，分为storageWithCacher和UndecodedStorage
	Decorator               StorageDecorator
	DeleteCollectionWorkers int

	ResourcePrefix string
}
```

###scheme 记录GroupVersionKind和type的映射关系
重要接口：
1. addKnownTypes　
2. addDefaultFuncs
3. addConversionFuncs    
4. AddFieldLabelConversionFunc　　　field label?

apiserver公用一个scheme,在api/register.go


每个group对应一个apiGroupInfo

ApiGroupVersion





```go
APIGroupVersion {
   Storage []rest.Storage

   Root GroupVersion unversioned.GroupVersion

   RequestInfoResolver *RequestInfoResolver

   OptionsExternalVersion *unversioned.GroupVersion

   Mapper meta.RESTMapper

   Serializer     runtime.NegotiatedSerializer
   ParameterCodec runtime.ParameterCodec

   Typer     runtime.ObjectTyper
   Creater   runtime.ObjectCreater
   Convertor runtime.ObjectConvertor
   Copier    runtime.ObjectCopier
   Linker    runtime.SelfLinker

   Admit   admission.Interface
   Context api.RequestContextMapper

   MinRequestTimeout time.Duration

   SubresourceGroupVersionKind []unversioned.GroupVersionKind

   ResourceLister APIResourceLister
}
```



##碰到的问题：
1. v1apha1 type 生成的pb混杂了很多其他的type
应该用v1.PodTemplate而不是api.PodTemplate
2. apiserver不能生成相应的对象
通过 update-codecgen.sh生成对应的序列化和反序列化函数
3. 不能自动生成conversion函数
cp gaia/types.go　gaia/v1apha1/types.go 并做相应修改，生成conversion时参考了comment?
4. error validating "tapp.yaml": error validating data: field templatePool: is required; if you choose to ignore these errors, turn validation off with --validate=false
kubectl 会从apiserver下载swagger　scheme文件，templatePool以前没有加omitEmpty，加了之后需要执行update命令生成swagger文件，并清楚~/.kube/schema







