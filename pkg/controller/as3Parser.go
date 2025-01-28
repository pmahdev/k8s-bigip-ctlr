package controller

import (
	"encoding/json"
	"fmt"
	log "github.com/F5Networks/k8s-bigip-ctlr/v2/pkg/vlogger"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// can you figure out the as3parser struct from this code of this entire file?

// NewAS3Parser creates a new AS3Parser instance
func NewAS3Parser(params AgentParams) *AS3Parser {
	return &AS3Parser{}
}

// AS3Parser interface defines methods for parsing AS3 declarations
type AS3ParserInterface interface {
	// Get deleted tenant declaration
	getDeletedTenantDeclaration(defaultPartition, tenant, cisLabel string, config *ResourceConfigRequest) as3Tenant

	// Process iRules for AS3
	processIRulesForAS3(rsMap ResourceMap, sharedApp as3Application)

	// Process data group for AS3
	processDataGroupForAS3(rsMap ResourceMap, sharedApp as3Application)

	// Process resources for AS3
	processResourcesForAS3(rsMap ResourceMap, sharedApp as3Application, shareNodes bool, tenant, poolMemberType string, bigipAs3Version float64)

	// Create policies declaration
	createPoliciesDecl(cfg *ResourceConfig, sharedApp as3Application)

	// Create pool declaration
	createPoolDecl(cfg *ResourceConfig, sharedApp as3Application, shareNodes bool, tenant, poolMemberType string)

	// Process iRules for CRD
	processIrulesForCRD(cfg *ResourceConfig, svc *as3Service)

	// Create service declaration
	createServiceDecl(cfg *ResourceConfig, sharedApp as3Application, tenant string, bigipAs3Version float64)

	// Create transport service declaration
	createTransportServiceDecl(cfg *ResourceConfig, sharedApp as3Application, tenant string)

	// Process common declaration
	processCommonDecl(cfg *ResourceConfig, svc *as3Service)

	// Create service address declaration
	createServiceAddressDecl(cfg *ResourceConfig, virtualAddress string, sharedApp as3Application) string

	// Create rule condition
	createRuleCondition(rl *Rule, rulesData *as3Rule, port int)

	// Create rule action
	createRuleAction(rl *Rule, rulesData *as3Rule)

	// Extract virtual address and port
	extractVirtualAddressAndPort(str string) (string, int)

	// Deep equal JSON
	DeepEqualJSON(decl1, decl2 as3Declaration) bool

	// Process profiles for AS3
	processProfilesForAS3(rsMap ResourceMap, sharedApp as3Application)

	// Process TLS profiles for AS3
	processTLSProfilesForAS3(cfg *ResourceConfig, svc *as3Service, profileName string)

	// Process custom profiles for AS3
	processCustomProfilesForAS3(rsMap ResourceMap, sharedApp as3Application, as3Version float64)

	// Create/Update TLS server
	createUpdateTLSServer(resourceType string, prof CustomProfile, svcName string, sharedApp as3Application) bool

	// Create certificate declaration
	createCertificateDecl(prof CustomProfile, sharedApp as3Application)

	// Create/Update CA bundle
	createUpdateCABundle(prof CustomProfile, sharedApp as3Application)

	// Create/Update TLS profile
	createUpdateTLSProfile(prof CustomProfile, sharedApp as3Application)

	// Create/Update TLS client profile
	createUpdateTLSClientProfile(prof CustomProfile, sharedApp as3Application)

	// Create monitor declaration
	createMonitorDecl(cfg *ResourceConfig, sharedApp as3Application)

	// Create TLS client
	createTLSClient(resourceType string, prof CustomProfile, svcName, caBundleName string, sharedApp as3Application) *as3TLSClient

	// Get sorted custom profile keys
	getSortedCustomProfileKeys(customProfiles map[SecretKey]CustomProfile) []SecretKey
}

func (ap *AS3Parser) getDeletedTenantDeclaration(defaultPartition, tenant, cisLabel string, config *ResourceConfigRequest) as3Tenant {
	if defaultPartition == tenant {
		// Flush Partition contents
		sharedApp := as3Application{}
		sharedApp["class"] = "Application"
		sharedApp["template"] = "shared"
		return as3Tenant{
			"class":              "Tenant",
			as3SharedApplication: sharedApp,
			"defaultRouteDomain": config.defaultRouteDomain,
			"label":              cisLabel,
		}
	}
	return as3Tenant{
		"class": "Tenant",
	}
}

func (ap *AS3Parser) processIRulesForAS3(rsMap ResourceMap, sharedApp as3Application) {
	for _, rsCfg := range rsMap {
		// Skip processing IRules for "None" value
		for _, v := range rsCfg.Virtual.IRules {
			if v == "none" {
				continue
			}
		}
		// Create irule declaration
		for _, v := range rsCfg.IRulesMap {
			iRule := &as3IRules{}
			iRule.Class = "iRule"
			iRule.IRule = v.Code
			sharedApp[v.Name] = iRule
		}
	}
}

func (ap *AS3Parser) processDataGroupForAS3(rsMap ResourceMap, sharedApp as3Application) {
	for _, rsCfg := range rsMap {
		// Skip processing DataGroup for "None" iRule value
		for _, v := range rsCfg.Virtual.IRules {
			if v == "none" {
				continue
			}
		}
		for _, idg := range rsCfg.IntDgMap {
			for _, dg := range idg {
				dataGroupRecord, found := sharedApp[dg.Name]
				if !found {
					dgMap := &as3DataGroup{}
					dgMap.Class = "Data_Group"
					dgMap.KeyDataType = dg.Type
					for _, record := range dg.Records {
						dgMap.Records = append(dgMap.Records, as3Record{Key: record.Name, Value: record.Data})
					}
					// sort above create dgMap records.
					sort.Slice(dgMap.Records, func(i, j int) bool { return (dgMap.Records[i].Key < dgMap.Records[j].Key) })
					sharedApp[dg.Name] = dgMap
				} else {
					for _, record := range dg.Records {
						sharedApp[dg.Name].(*as3DataGroup).Records = append(dataGroupRecord.(*as3DataGroup).Records, as3Record{Key: record.Name, Value: record.Data})
					}
					// sort above created
					sort.Slice(sharedApp[dg.Name].(*as3DataGroup).Records,
						func(i, j int) bool {
							return (sharedApp[dg.Name].(*as3DataGroup).Records[i].Key <
								sharedApp[dg.Name].(*as3DataGroup).Records[j].Key)
						})
				}
			}
		}
	}
}

// Process for AS3 Resource
func (ap *AS3Parser) processResourcesForAS3(rsMap ResourceMap, sharedApp as3Application, shareNodes bool, tenant, poolMemberType string) {
	for _, cfg := range rsMap {
		//Create policies
		ap.createPoliciesDecl(cfg, sharedApp)

		//Create health monitor declaration
		ap.createMonitorDecl(cfg, sharedApp)

		//Create pools
		ap.createPoolDecl(cfg, sharedApp, shareNodes, tenant, poolMemberType)

		switch cfg.MetaData.ResourceType {
		case VirtualServer:
			//Create AS3 Service for virtual server
			ap.createServiceDecl(cfg, sharedApp, tenant, ap.bigIPAS3Version)
		case TransportServer:
			//Create AS3 Service for transport virtual server
			ap.createTransportServiceDecl(cfg, sharedApp, tenant)
		}
	}
}

// Create policy declaration
func (ap *AS3Parser) createPoliciesDecl(cfg *ResourceConfig, sharedApp as3Application) {
	_, port := ap.extractVirtualAddressAndPort(cfg.Virtual.Destination)
	for _, pl := range cfg.Policies {
		//Create EndpointPolicy
		ep := &as3EndpointPolicy{}
		for _, rl := range pl.Rules {

			ep.Class = "Endpoint_Policy"
			s := strings.Split(pl.Strategy, "/")
			ep.Strategy = s[len(s)-1]

			//Create rules
			rulesData := &as3Rule{Name: rl.Name}

			//Create condition object
			ap.createRuleCondition(rl, rulesData, port)

			//Create action object
			ap.createRuleAction(rl, rulesData)

			ep.Rules = append(ep.Rules, rulesData)
		}
		//Setting Endpoint_Policy Name
		sharedApp[pl.Name] = ep
	}
}

// Create AS3 Pools for CRD
func (ap *AS3Parser) createPoolDecl(cfg *ResourceConfig, sharedApp as3Application, shareNodes bool, tenant, poolMemberType string) {
	for _, v := range cfg.Pools {
		pool := &as3Pool{}
		pool.LoadBalancingMode = v.Balance
		pool.Class = "Pool"
		pool.ReselectTries = v.ReselectTries
		pool.ServiceDownAction = v.ServiceDownAction
		pool.SlowRampTime = v.SlowRampTime
		poolMemberSet := make(map[PoolMember]struct{})
		for _, val := range v.Members {
			// Skip duplicate pool members
			if _, ok := poolMemberSet[val]; ok {
				continue
			}
			poolMemberSet[val] = struct{}{}
			var member as3PoolMember
			member.AddressDiscovery = "static"
			member.ServicePort = val.Port
			if val.Ratio > 0 {
				member.Ratio = val.Ratio
			}
			member.ServerAddresses = append(member.ServerAddresses, val.Address)
			if shareNodes || (poolMemberType == Auto && val.MemberType == NodePort) {
				member.ShareNodes = true
			}
			if val.AdminState != "" {
				member.AdminState = val.AdminState
			}
			if val.ConnectionLimit != 0 {
				member.ConnectionLimit = val.ConnectionLimit
			}
			pool.Members = append(pool.Members, member)
		}
		for _, val := range v.MonitorNames {
			var monitor as3ResourcePointer
			//Reference existing health monitor from BIGIP
			if val.Reference == BIGIP {
				monitor.BigIP = val.Name
			} else {
				use := strings.Split(val.Name, "/")
				monitor.Use = fmt.Sprintf("/%s/%s/%s",
					tenant,
					as3SharedApplication,
					use[len(use)-1],
				)
			}
			pool.Monitors = append(pool.Monitors, monitor)
		}
		if len(pool.Monitors) > 0 {
			if v.MinimumMonitors.StrVal != "" || v.MinimumMonitors.IntVal != 0 {
				pool.MinimumMonitors = v.MinimumMonitors
			} else {
				pool.MinimumMonitors = intstr.IntOrString{Type: 0, IntVal: 1}
			}
		} else {
			pool.MinimumMonitors = intstr.IntOrString{Type: 1, StrVal: "all"}
		}
		if pl, ok := sharedApp[v.Name]; ok {
			if pl.(*as3Pool) != nil && len(pl.(*as3Pool).Monitors) > 0 {
				for _, mon := range pl.(*as3Pool).Monitors {
					exist := false
					for _, plMon := range pool.Monitors {
						if reflect.DeepEqual(mon, plMon) {
							exist = true
							break
						}
					}
					if !exist {
						pool.Monitors = append(pool.Monitors, mon)
					}
				}
			}
		}
		sharedApp[v.Name] = pool
	}
}

func (ap *AS3Parser) updateVirtualToHTTPS(v *as3Service) {
	v.Class = "Service_HTTPS"
	redirect80 := false
	v.Redirect80 = &redirect80
}

// Process Irules for CRD
func (ap *AS3Parser) processIrulesForCRD(cfg *ResourceConfig, svc *as3Service) {
	var IRules []interface{}
	// Skip processing IRules for "None" value
	for _, v := range cfg.Virtual.IRules {
		if v == "none" {
			continue
		}
		splits := strings.Split(v, "/")
		iRuleName := splits[len(splits)-1]

		var iRuleNoPort string
		lastIndex := strings.LastIndex(iRuleName, "_")
		if lastIndex > 0 {
			iRuleNoPort = iRuleName[:lastIndex]
		} else {
			iRuleNoPort = iRuleName
		}
		if strings.HasSuffix(iRuleNoPort, HttpRedirectIRuleName) ||
			strings.HasSuffix(iRuleNoPort, HttpRedirectNoHostIRuleName) ||
			strings.HasSuffix(iRuleName, TLSIRuleName) ||
			strings.HasSuffix(iRuleName, ABPathIRuleName) {

			IRules = append(IRules, iRuleName)
		} else {
			irule := &as3ResourcePointer{
				BigIP: v,
			}
			IRules = append(IRules, irule)
		}
		svc.IRules = IRules
	}
}

// Create AS3 Service for CRD
func (ap *AS3Parser) createServiceDecl(cfg *ResourceConfig, sharedApp as3Application, tenant string, bigipAs3Version float64) {
	svc := &as3Service{}
	numPolicies := len(cfg.Virtual.Policies)
	switch {
	case numPolicies == 1:
		policyName := cfg.Virtual.Policies[0].Name
		svc.PolicyEndpoint = fmt.Sprintf("/%s/%s/%s",
			tenant,
			as3SharedApplication,
			policyName)
	case numPolicies > 1:
		var peps []as3ResourcePointer
		for _, pep := range cfg.Virtual.Policies {
			peps = append(
				peps,
				as3ResourcePointer{
					Use: fmt.Sprintf("/%s/%s/%s",
						tenant,
						as3SharedApplication,
						pep.Name,
					),
				},
			)
		}
		svc.PolicyEndpoint = peps
	}
	// Attach the default pool if pool name is present for virtual.
	if cfg.Virtual.PoolName != "" {
		var poolPointer as3ResourcePointer
		if cfg.MetaData.defaultPoolType == BIGIP {
			poolPointer.BigIP = cfg.Virtual.PoolName
		} else {
			ps := strings.Split(cfg.Virtual.PoolName, "/")
			poolPointer.Use = fmt.Sprintf("/%s/%s/%s",
				tenant,
				as3SharedApplication,
				ps[len(ps)-1],
			)
		}
		svc.Pool = &poolPointer
	}

	if cfg.Virtual.TLSTermination != TLSPassthrough {
		svc.Layer4 = cfg.Virtual.IpProtocol
		svc.Source = "0.0.0.0/0"
		svc.TranslateServerAddress = true
		svc.TranslateServerPort = true
		svc.Class = "Service_HTTP"
	} else {
		if len(cfg.Virtual.PersistenceProfile) == 0 {
			cfg.Virtual.PersistenceProfile = "tls-session-id"
		}
		svc.Class = "Service_TCP"
	}

	svc.addPersistenceMethod(cfg.Virtual.PersistenceProfile)

	if len(cfg.Virtual.ProfileDOS) > 0 {
		svc.ProfileDOS = &as3ResourcePointer{
			BigIP: cfg.Virtual.ProfileDOS,
		}
	}
	if len(cfg.Virtual.ProfileBotDefense) > 0 {
		svc.ProfileBotDefense = &as3ResourcePointer{
			BigIP: cfg.Virtual.ProfileBotDefense,
		}
	}
	if len(cfg.Virtual.HTMLProfile) > 0 {
		svc.ProfileHTML = &as3ResourcePointer{
			BigIP: cfg.Virtual.HTMLProfile,
		}
	}

	if len(cfg.Virtual.HTTPCompressionProfile) > 0 {
		if cfg.Virtual.HTTPCompressionProfile == "basic" || cfg.Virtual.HTTPCompressionProfile == "wan" {
			svc.HTTPCompressionProfile = cfg.Virtual.HTTPCompressionProfile
		} else {
			svc.HTTPCompressionProfile = &as3ResourcePointer{
				BigIP: cfg.Virtual.HTTPCompressionProfile,
			}
		}
	}

	if cfg.MetaData.Protocol == "https" {
		if len(cfg.Virtual.HTTP2.Client) > 0 || len(cfg.Virtual.HTTP2.Server) > 0 {
			if cfg.Virtual.HTTP2.Client == "" {
				log.Errorf("[AS3] resetting ProfileHTTP2 as client profile doesnt co-exist with HTTP2 Server Profile, Please include client HTTP2 Profile ")
			}
			if cfg.Virtual.HTTP2.Server == "" {
				svc.ProfileHTTP2 = &as3ResourcePointer{
					BigIP: fmt.Sprintf("%v", cfg.Virtual.HTTP2.Client),
				}
			}
			if cfg.Virtual.HTTP2.Client == "" && cfg.Virtual.HTTP2.Server != "" {
				svc.ProfileHTTP2 = as3ProfileHTTP2{
					Egress: &as3ResourcePointer{
						BigIP: fmt.Sprintf("%v", cfg.Virtual.HTTP2.Server),
					},
				}
			}
			if cfg.Virtual.HTTP2.Client != "" && cfg.Virtual.HTTP2.Server != "" {
				svc.ProfileHTTP2 = as3ProfileHTTP2{
					Ingress: &as3ResourcePointer{
						BigIP: fmt.Sprintf("%v", cfg.Virtual.HTTP2.Client),
					},
					Egress: &as3ResourcePointer{
						BigIP: fmt.Sprintf("%v", cfg.Virtual.HTTP2.Server),
					},
				}
			}
		}
	}

	if len(cfg.Virtual.TCP.Client) > 0 || len(cfg.Virtual.TCP.Server) > 0 {
		if cfg.Virtual.TCP.Client == "" {
			log.Errorf("[AS3] resetting ProfileTCP as client profile doesnt co-exist with TCP Server Profile, Please include client TCP Profile ")
		}
		if cfg.Virtual.TCP.Server == "" {
			svc.ProfileTCP = &as3ResourcePointer{
				BigIP: fmt.Sprintf("%v", cfg.Virtual.TCP.Client),
			}
		}
		if cfg.Virtual.TCP.Client != "" && cfg.Virtual.TCP.Server != "" {
			svc.ProfileTCP = as3ProfileTCP{
				Ingress: &as3ResourcePointer{
					BigIP: fmt.Sprintf("%v", cfg.Virtual.TCP.Client),
				},
				Egress: &as3ResourcePointer{
					BigIP: fmt.Sprintf("%v", cfg.Virtual.TCP.Server),
				},
			}
		}
	}

	if len(cfg.Virtual.ProfileMultiplex) > 0 {
		svc.ProfileMultiplex = &as3ResourcePointer{
			BigIP: cfg.Virtual.ProfileMultiplex,
		}
	}
	// updating the virtual server to https if a passthrough datagroup is found
	name := getRSCfgResName(cfg.Virtual.Name, PassthroughHostsDgName)
	mapKey := NameRef{
		Name:      name,
		Partition: cfg.Virtual.Partition,
	}
	if _, ok := cfg.IntDgMap[mapKey]; ok {
		if bigipAs3Version < 3.52 {
			svc.ServerTLS = &as3ResourcePointer{
				BigIP: "/Common/clientssl",
			}
		}
		ap.updateVirtualToHTTPS(svc)
	}

	// Attaching Profiles from Policy CRD
	for _, profile := range cfg.Virtual.Profiles {
		_, name := getPartitionAndName(profile.Name)
		switch profile.Context {
		case "http":
			if !profile.BigIPProfile {
				svc.ProfileHTTP = name
			} else {
				svc.ProfileHTTP = &as3ResourcePointer{
					BigIP: fmt.Sprintf("%v", profile.Name),
				}
			}
		}
	}

	//Attaching WAF policy
	if cfg.Virtual.WAF != "" {
		svc.WAF = &as3ResourcePointer{
			BigIP: fmt.Sprintf("%v", cfg.Virtual.WAF),
		}
	}

	virtualAddress, port := ap.extractVirtualAddressAndPort(cfg.Virtual.Destination)
	// verify that ip address and port exists.
	if virtualAddress != "" && port != 0 {
		if len(cfg.ServiceAddress) == 0 {
			va := append(svc.VirtualAddresses, virtualAddress)
			if len(cfg.Virtual.AdditionalVirtualAddresses) > 0 {
				for _, val := range cfg.Virtual.AdditionalVirtualAddresses {
					if cfg.Virtual.BigIPRouteDomain > 0 {
						val = fmt.Sprintf("%s%%%d", val, cfg.Virtual.BigIPRouteDomain)
					}
					va = append(va, val)
				}
			}
			svc.VirtualAddresses = va
			svc.VirtualPort = port
		} else {
			//Attach Service Address
			serviceAddressName := ap.createServiceAddressDecl(cfg, virtualAddress, sharedApp)
			sa := &as3ResourcePointer{
				Use: serviceAddressName,
			}
			svc.VirtualAddresses = append(svc.VirtualAddresses, sa)
			if len(cfg.Virtual.AdditionalVirtualAddresses) > 0 {
				for _, val := range cfg.Virtual.AdditionalVirtualAddresses {
					if cfg.Virtual.BigIPRouteDomain > 0 {
						val = fmt.Sprintf("%s%%%d", val, cfg.Virtual.BigIPRouteDomain)
					}
					//Attach Service Address
					serviceAddressName := ap.createServiceAddressDecl(cfg, val, sharedApp)
					//handle additional service addresses
					asa := &as3ResourcePointer{
						Use: serviceAddressName,
					}
					svc.VirtualAddresses = append(svc.VirtualAddresses, asa)
				}
			}
			svc.VirtualPort = port
		}
	}
	if cfg.Virtual.HttpMrfRoutingEnabled != nil {
		//set HttpMrfRoutingEnabled
		svc.HttpMrfRoutingEnabled = *cfg.Virtual.HttpMrfRoutingEnabled
	}
	svc.AutoLastHop = cfg.Virtual.AutoLastHop

	if cfg.Virtual.AnalyticsProfiles.HTTPAnalyticsProfile != "" {
		svc.HttpAnalyticsProfile = &as3ResourcePointer{
			BigIP: cfg.Virtual.AnalyticsProfiles.HTTPAnalyticsProfile,
		}
	}
	//set websocket profile
	if cfg.Virtual.ProfileWebSocket != "" {
		svc.ProfileWebSocket = &as3ResourcePointer{
			BigIP: cfg.Virtual.ProfileWebSocket,
		}
	}
	ap.processCommonDecl(cfg, svc)
	sharedApp[cfg.Virtual.Name] = svc
}

// Create AS3 Service Address for Virtual Server Address
func (ap *AS3Parser) createServiceAddressDecl(cfg *ResourceConfig, virtualAddress string, sharedApp as3Application) string {
	var name string
	for _, sa := range cfg.ServiceAddress {
		serviceAddress := &as3ServiceAddress{}
		serviceAddress.Class = "Service_Address"
		serviceAddress.ArpEnabled = sa.ArpEnabled
		serviceAddress.ICMPEcho = sa.ICMPEcho
		serviceAddress.RouteAdvertisement = sa.RouteAdvertisement
		serviceAddress.SpanningEnabled = sa.SpanningEnabled
		serviceAddress.TrafficGroup = sa.TrafficGroup
		serviceAddress.VirtualAddress = virtualAddress
		name = "crd_service_address_" + AS3NameFormatter(virtualAddress)
		sharedApp[name] = serviceAddress
	}
	return name
}

// Create AS3 Rule Condition for CRD
func (ap *AS3Parser) createRuleCondition(rl *Rule, rulesData *as3Rule, port int) {
	for _, c := range rl.Conditions {
		condition := &as3Condition{}

		if c.Host {
			condition.Name = "host"
			var values []string
			// For ports other then 80 and 443, attaching port number to host.
			// Ex. example.com:8080
			if port != 80 && port != 443 {
				for i := range c.Values {
					val := c.Values[i] + ":" + strconv.Itoa(port)
					values = append(values, val)
				}
			} else {
				//For ports 80 and 443, host header should match both
				// host and host:port match
				for i := range c.Values {
					val := c.Values[i] + ":" + strconv.Itoa(port)
					values = append(values, val, c.Values[i])
				}
			}
			condition.All = &as3PolicyCompareString{
				Values: values,
			}
			if c.HTTPHost {
				condition.Type = "httpHeader"
			}
			if c.Equals {
				condition.All.Operand = "equals"
			}
			if c.EndsWith {
				condition.All.Operand = "ends-with"
			}
		} else if c.PathSegment {
			condition.PathSegment = &as3PolicyCompareString{
				Values: c.Values,
			}
			if c.Name != "" {
				condition.Name = c.Name
			}
			condition.Index = c.Index
			if c.HTTPURI {
				condition.Type = "httpUri"
			}
			if c.Equals {
				condition.PathSegment.Operand = "equals"
			}
		} else if c.Path {
			condition.Path = &as3PolicyCompareString{
				Values: c.Values,
			}
			if c.Name != "" {
				condition.Name = c.Name
			}
			condition.Index = c.Index
			if c.HTTPURI {
				condition.Type = "httpUri"
			}
			if c.Equals {
				condition.Path.Operand = "equals"
			}
		} else if c.Tcp {
			if c.Address && len(c.Values) > 0 {
				condition.Type = "tcp"
				condition.Address = &as3PolicyAddressString{
					Values: c.Values,
				}
			}
		}
		if c.Request {
			condition.Event = "request"
		}

		rulesData.Conditions = append(rulesData.Conditions, condition)
	}
}

// Create AS3 Rule Action for CRD
func (ap *AS3Parser) createRuleAction(rl *Rule, rulesData *as3Rule) {
	for _, v := range rl.Actions {
		action := &as3Action{}
		if v.Forward {
			action.Type = "forward"
		}
		if v.Log {
			action.Type = "log"
		}
		if v.Request {
			action.Event = "request"
		}
		if v.Redirect {
			action.Type = "httpRedirect"
		}
		if v.HTTPHost {
			action.Type = "httpHeader"
		}
		if v.HTTPURI {
			action.Type = "httpUri"
		}
		if v.Location != "" {
			action.Location = v.Location
		}
		if v.Log {
			action.Write = &as3LogMessage{
				Message: v.Message,
			}
		}
		// Handle vsHostname rewrite.
		if v.Replace && v.HTTPHost {
			action.Replace = &as3ActionReplaceMap{
				Value: v.Value,
				Name:  "host",
			}
		}
		// handle uri rewrite.
		if v.Replace && v.HTTPURI {
			action.Replace = &as3ActionReplaceMap{
				Value: v.Value,
			}
		}
		p := strings.Split(v.Pool, "/")
		if v.Pool != "" {
			action.Select = &as3ActionForwardSelect{
				Pool: &as3ResourcePointer{
					Use: p[len(p)-1],
				},
			}
		}
		// WAF action
		if v.WAF {
			action.Type = "waf"
		}
		// Add policy reference
		if v.Policy != "" {
			action.Policy = &as3ResourcePointer{
				BigIP: v.Policy,
			}
		}
		if v.Enabled != nil {
			action.Enabled = v.Enabled
		}
		// Add drop action if specified
		if v.Drop {
			action.Type = "drop"
		}

		if v.PersistMethod != "" {
			switch v.PersistMethod {
			case SourceAddress:
				action.Event = "request"
				action.Type = "persist"
				action.SourceAddress = &PersistMetaData{
					Netmask: v.Netmask,
					Timeout: v.Timeout,
				}
			case DestinationAddress:
				action.Event = "request"
				action.Type = "persist"
				action.DestinationAddress = &PersistMetaData{
					Netmask: v.Netmask,
					Timeout: v.Timeout,
				}
			case CookieHash:
				action.Event = "request"
				action.Type = "persist"
				action.CookieHash = &PersistMetaData{
					Timeout: v.Timeout,
					Offset:  v.Offset,
					Length:  v.Length,
					Name:    v.Name,
				}
			case CookieInsert:
				action.Event = "request"
				action.Type = "persist"
				action.CookieInsert = &PersistMetaData{
					Name:   v.Name,
					Expiry: v.Expiry,
				}
			case CookieRewrite:
				action.Event = "request"
				action.Type = "persist"
				action.CookieRewrite = &PersistMetaData{
					Name:   v.Name,
					Expiry: v.Expiry,
				}
			case CookiePassive:
				action.Event = "request"
				action.Type = "persist"
				action.CookiePassive = &PersistMetaData{
					Name: v.Name,
				}
			case Universal:
				action.Event = "request"
				action.Type = "persist"
				action.Universal = &PersistMetaData{
					Key:     v.Key,
					Timeout: v.Timeout,
				}
			case Carp:
				action.Event = "request"
				action.Type = "persist"
				action.Carp = &PersistMetaData{
					Key:     v.Key,
					Timeout: v.Timeout,
				}
			case Hash:
				action.Event = "request"
				action.Type = "persist"
				action.Hash = &PersistMetaData{
					Key:     v.Key,
					Timeout: v.Timeout,
				}
			case Disable:
				action.Event = "request"
				action.Type = "persist"
				action.Disable = &PersistMetaData{}
			default:
				log.Warning("provide a persist method value from sourceAddress, destinationAddress, cookieInsert, cookieRewrite, cookiePassive, cookieHash, universal, hash, and carp")
			}
		}

		rulesData.Actions = append(rulesData.Actions, action)
	}
}

// Extract virtual address and port from host URL
func (ap *AS3Parser) extractVirtualAddressAndPort(str string) (string, int) {

	destination := strings.Split(str, "/")
	// split separator is in accordance with SetVirtualAddress func (ap *AS3Parser)tion - ipv4/6 format
	ipPort := strings.Split(destination[len(destination)-1], ":")
	if len(ipPort) != 2 {
		ipPort = strings.Split(destination[len(destination)-1], ".")
	}
	// verify that ip address and port exists else log error.
	if len(ipPort) == 2 {
		port, _ := strconv.Atoi(ipPort[1])
		return ipPort[0], port
	} else {
		log.Error("Invalid Virtual Server Destination IP address/Port.")
		return "", 0
	}

}

func (ap *AS3Parser) DeepEqualJSON(decl1, decl2 as3Declaration) bool {
	if decl1 == "" && decl2 == "" {
		return true
	}
	var o1, o2 interface{}

	err := json.Unmarshal([]byte(decl1), &o1)
	if err != nil {
		return false
	}

	err = json.Unmarshal([]byte(decl2), &o2)
	if err != nil {
		return false
	}

	return reflect.DeepEqual(o1, o2)
}

func (ap *AS3Parser) processProfilesForAS3(rsMap ResourceMap, sharedApp as3Application) {
	for _, cfg := range rsMap {
		if svc, ok := sharedApp[cfg.Virtual.Name].(*as3Service); ok {
			ap.processTLSProfilesForAS3(cfg, svc, cfg.Virtual.Name)
		}
	}
}

func (ap *AS3Parser) processTLSProfilesForAS3(cfg *ResourceConfig, svc *as3Service, profileName string) {
	// lets discard BIGIP profile creation when there exists a custom profile.
	virtual := cfg.Virtual
	as3ClientSuffix := "_tls_client"
	as3ServerSuffix := "_tls_server"
	var clientProfiles []as3MultiTypeParam
	var serverProfiles []as3MultiTypeParam
	for _, profile := range virtual.Profiles {
		switch profile.Context {
		case CustomProfileClient:
			// Profile is stored in a k8s secret
			if !profile.BigIPProfile {
				// Incoming traffic (clientssl) from a web client will be handled by ServerTLS in AS3
				svc.ServerTLS = fmt.Sprintf("/%v/%v/%v%v", virtual.Partition,
					as3SharedApplication, profileName, as3ServerSuffix)

			} else {
				// Profile is a BIG-IP reference
				// Incoming traffic (clientssl) from a web client will be handled by ServerTLS in AS3
				clientProfiles = append(clientProfiles, &as3ResourcePointer{
					BigIP: fmt.Sprintf("/%v/%v", profile.Partition, profile.Name),
				})
			}
			if cfg.MetaData.ResourceType == VirtualServer {
				updateVirtualToHTTPS(svc)
			}
		case CustomProfileServer:
			// Profile is stored in a k8s secret
			if !profile.BigIPProfile {
				// Outgoing traffic (serverssl) to BackEnd Servers from BigIP will be handled by ClientTLS in AS3
				svc.ClientTLS = fmt.Sprintf("/%v/%v/%v%v", virtual.Partition,
					as3SharedApplication, profileName, as3ClientSuffix)
			} else {
				// Profile is a BIG-IP reference
				// Outgoing traffic (serverssl) to BackEnd Servers from BigIP will be handled by ClientTLS in AS3
				serverProfiles = append(serverProfiles, &as3ResourcePointer{
					BigIP: fmt.Sprintf("/%v/%v", profile.Partition, profile.Name),
				})
			}
			if cfg.MetaData.ResourceType == VirtualServer {
				updateVirtualToHTTPS(svc)
			}
		}
	}
	if len(clientProfiles) > 0 {
		svc.ServerTLS = clientProfiles
	}
	if len(serverProfiles) > 0 {
		svc.ClientTLS = serverProfiles
	}
}

func (ap *AS3Parser) processCustomProfilesForAS3(rsMap ResourceMap, sharedApp as3Application) {
	caBundleName := "serverssl_ca_bundle"
	var tlsClient *as3TLSClient
	svcNameMap := make(map[string]struct{})
	// TLS Certificates are available in CustomProfiles
	for _, rsCfg := range rsMap {
		// Sort customProfiles so that they are processed in orderly manner
		keys := ap.getSortedCustomProfileKeys(rsCfg.customProfiles)

		for _, key := range keys {
			prof := rsCfg.customProfiles[key]
			// Create TLSServer and Certificate for each profile
			svcName := key.ResourceName
			if svcName == "" {
				continue
			}
			if ok := ap.createUpdateTLSServer(rsCfg.MetaData.ResourceType, prof, svcName, sharedApp); ok {
				// Create Certificate only if the corresponding TLSServer is created
				ap.createCertificateDecl(prof, sharedApp)
				svcNameMap[svcName] = struct{}{}
			} else {
				ap.createUpdateCABundle(prof, caBundleName, sharedApp)
				tlsClient = ap.createTLSClient(rsCfg.MetaData.ResourceType, prof, svcName, caBundleName, sharedApp)

				skey := SecretKey{
					Name: prof.Name + "-ca",
				}
				if _, ok := rsCfg.customProfiles[skey]; ok && tlsClient != nil {
					// If a profile exist in customProfiles with key as created above
					// then it indicates that secure-serverssl needs to be added
					tlsClient.ValidateCertificate = true
				}
			}
		}
	}
	// if AS3 version on bigIP is lower than 3.44 then don't enable sniDefault, as it's only supported from AS3 v3.44 onwards
	if ap.AS3VersionInfo.as3Version < 3.44 {
		return
	}
	for svcName, _ := range svcNameMap {
		if _, ok := sharedApp[svcName].(*as3Service); ok {
			tlsServerName := fmt.Sprintf("%s_tls_server", svcName)
			tlsServer, ok := sharedApp[tlsServerName].(*as3TLSServer)
			if !ok {
				continue
			}
			if len(tlsServer.Certificates) > 1 {
				tlsServer.Certificates[0].SNIDefault = true
			}
		}
	}
}

// createUpdateTLSServer creates a new TLSServer instance or updates if one exists already
func (ap *AS3Parser) createUpdateTLSServer(resourceType string, prof CustomProfile, svcName string, sharedApp as3Application) bool {
	if len(prof.Certificates) > 0 {
		if sharedApp[svcName] == nil {
			return false
		}
		svc := sharedApp[svcName].(*as3Service)
		tlsServerName := fmt.Sprintf("%s_tls_server", svcName)
		tlsServer, ok := sharedApp[tlsServerName].(*as3TLSServer)
		if !ok {
			tlsServer = &as3TLSServer{
				Class:        "TLS_Server",
				Certificates: []as3TLSServerCertificates{},
			}
			if prof.TLS1_0Enabled != nil {
				tlsServer.TLS1_0Enabled = prof.TLS1_0Enabled
			}
			if prof.TLS1_1Enabled != nil {
				tlsServer.TLS1_1Enabled = prof.TLS1_1Enabled
			}
			if prof.TLS1_2Enabled != nil {
				tlsServer.TLS1_2Enabled = prof.TLS1_2Enabled
			}
			if prof.CipherGroup != "" {
				tlsServer.CipherGroup = &as3ResourcePointer{BigIP: prof.CipherGroup}
				tlsServer.TLS1_3Enabled = true
			} else {
				tlsServer.Ciphers = prof.Ciphers
			}
			if prof.RenegotiationEnabled != nil {
				tlsServer.RenegotiationEnabled = prof.RenegotiationEnabled
			}
			sharedApp[tlsServerName] = tlsServer
			svc.ServerTLS = tlsServerName
			if resourceType == VirtualServer {
				updateVirtualToHTTPS(svc)
			}
		}
		for index, certificate := range prof.Certificates {
			certName := fmt.Sprintf("%s_%d", prof.Name, index)
			// A TLSServer profile needs to carry both Certificate and Key
			if len(certificate.Cert) > 0 && len(certificate.Key) > 0 {
				tlsServer.Certificates = append(
					tlsServer.Certificates,
					as3TLSServerCertificates{
						Certificate: certName,
					},
				)
			} else {
				return false
			}
		}
		return true
	}
	return false
}

func (ap *AS3Parser) createCertificateDecl(prof CustomProfile, sharedApp as3Application) {
	for index, certificate := range prof.Certificates {
		if len(certificate.Cert) > 0 && len(certificate.Key) > 0 {
			cert := &as3Certificate{
				Class:       "Certificate",
				Certificate: certificate.Cert,
				PrivateKey:  certificate.Key,
				ChainCA:     prof.CAFile,
			}
			sharedApp[fmt.Sprintf("%s_%d", prof.Name, index)] = cert
		}
	}
}

func (ap *AS3Parser) createUpdateCABundle(prof CustomProfile, caBundleName string, sharedApp as3Application) {
	for _, cert := range prof.Certificates {
		// For TLSClient only Cert (DestinationCACertificate) is given and key is empty string
		if len(cert.Cert) > 0 && len(cert.Key) == 0 {
			caBundle, ok := sharedApp[caBundleName].(*as3CABundle)

			if !ok {
				caBundle = &as3CABundle{
					Class:  "CA_Bundle",
					Bundle: "",
				}
				sharedApp[caBundleName] = caBundle
			}
			caBundle.Bundle += "\n" + cert.Cert
		}
	}
}

func (ap *AS3Parser) createTLSClient(
	resourceType string,
	prof CustomProfile,
	svcName, caBundleName string,
	sharedApp as3Application,
) *as3TLSClient {

	// For TLSClient only Cert (DestinationCACertificate) is given and key is empty string
	for _, certificate := range prof.Certificates {
		if certificate.Key != "" {
			return nil
		}
	}
	if _, ok := sharedApp[svcName]; len(prof.Certificates) > 0 && ok {
		svc := sharedApp[svcName].(*as3Service)
		tlsClientName := fmt.Sprintf("%s_tls_client", svcName)

		tlsClient := &as3TLSClient{
			Class: "TLS_Client",
			TrustCA: &as3ResourcePointer{
				Use: caBundleName,
			},
		}
		if prof.TLS1_0Enabled != nil {
			tlsClient.TLS1_0Enabled = prof.TLS1_0Enabled
		}
		if prof.TLS1_1Enabled != nil {
			tlsClient.TLS1_1Enabled = prof.TLS1_1Enabled
		}
		if prof.TLS1_2Enabled != nil {
			tlsClient.TLS1_2Enabled = prof.TLS1_2Enabled
		}
		if prof.CipherGroup != "" {
			tlsClient.CipherGroup = &as3ResourcePointer{BigIP: prof.CipherGroup}
			tlsClient.TLS1_3Enabled = true
		} else {
			tlsClient.Ciphers = prof.Ciphers
		}
		if prof.RenegotiationEnabled != nil {
			tlsClient.RenegotiationEnabled = prof.RenegotiationEnabled
		}
		sharedApp[tlsClientName] = tlsClient
		svc.ClientTLS = tlsClientName
		if resourceType == VirtualServer {
			updateVirtualToHTTPS(svc)
		}
		return tlsClient
	}
	return nil
}

// Create health monitor declaration
func (ap *AS3Parser) createMonitorDecl(cfg *ResourceConfig, sharedApp as3Application) {

	for _, v := range cfg.Monitors {
		monitor := &as3Monitor{}
		monitor.Class = "Monitor"
		monitor.Interval = v.Interval
		monitor.MonitorType = v.Type
		monitor.Timeout = v.Timeout
		val := 0
		monitor.TargetPort = v.TargetPort
		targetAddressStr := ""
		monitor.TargetAddress = &targetAddressStr
		monitor.TimeUnitilUp = v.TimeUntilUp
		//Monitor type
		switch v.Type {
		case "http":
			adaptiveFalse := false
			monitor.Adaptive = &adaptiveFalse
			monitor.Dscp = &val
			monitor.Receive = "none"
			if v.Recv != "" {
				monitor.Receive = v.Recv
			}
			monitor.Send = v.Send
		case "https":
			//Todo: For https monitor type
			adaptiveFalse := false
			monitor.Adaptive = &adaptiveFalse
			if v.Recv != "" {
				monitor.Receive = v.Recv
			}
			monitor.Send = v.Send
			monitor.TimeUnitilUp = v.TimeUntilUp
			if v.SSLProfile != "" {
				monitor.ClientTLS = &as3ResourcePointer{BigIP: fmt.Sprintf("%v", v.SSLProfile)}
			}
		case "tcp", "udp":
			adaptiveFalse := false
			monitor.Adaptive = &adaptiveFalse
			monitor.Receive = v.Recv
			monitor.Send = v.Send
		}
		sharedApp[v.Name] = monitor
	}

}

// Create AS3 transport Service for CRD
func (ap *AS3Parser) createTransportServiceDecl(cfg *ResourceConfig, sharedApp as3Application, tenant string) {
	svc := &as3Service{}
	if cfg.Virtual.Mode == "standard" {
		if cfg.Virtual.IpProtocol == "udp" {
			svc.Class = "Service_UDP"
		} else if cfg.Virtual.IpProtocol == "sctp" {
			svc.Class = "Service_SCTP"
		} else {
			svc.Class = "Service_TCP"
			//set ftp profile for only TCP
			if cfg.Virtual.FTPProfile != "" {
				svc.ProfileFTP = &as3ResourcePointer{
					BigIP: cfg.Virtual.FTPProfile,
				}
			}
		}
	} else if cfg.Virtual.Mode == "performance" {
		svc.Class = "Service_L4"
		if cfg.Virtual.IpProtocol == "udp" {
			svc.Layer4 = "udp"
		} else if cfg.Virtual.IpProtocol == "sctp" {
			svc.Layer4 = "sctp"
		} else {
			svc.Layer4 = "tcp"
		}
	}

	svc.ProfileL4 = "basic"
	if len(cfg.Virtual.ProfileL4) > 0 {
		svc.ProfileL4 = &as3ResourcePointer{
			BigIP: cfg.Virtual.ProfileL4,
		}
	}

	svc.addPersistenceMethod(cfg.Virtual.PersistenceProfile)

	if len(cfg.Virtual.ProfileDOS) > 0 {
		svc.ProfileDOS = &as3ResourcePointer{
			BigIP: cfg.Virtual.ProfileDOS,
		}
	}

	if len(cfg.Virtual.ProfileBotDefense) > 0 {
		svc.ProfileBotDefense = &as3ResourcePointer{
			BigIP: cfg.Virtual.ProfileBotDefense,
		}
	}

	if len(cfg.Virtual.TCP.Client) > 0 || len(cfg.Virtual.TCP.Server) > 0 {
		if cfg.Virtual.TCP.Client == "" {
			log.Errorf("[AS3] resetting ProfileTCP as client profile doesnt co-exist with TCP Server Profile, Please include client TCP Profile ")
		}
		if cfg.Virtual.TCP.Server == "" {
			svc.ProfileTCP = &as3ResourcePointer{
				BigIP: fmt.Sprintf("%v", cfg.Virtual.TCP.Client),
			}
		}
		if cfg.Virtual.TCP.Client != "" && cfg.Virtual.TCP.Server != "" {
			svc.ProfileTCP = as3ProfileTCP{
				Ingress: &as3ResourcePointer{
					BigIP: fmt.Sprintf("%v", cfg.Virtual.TCP.Client),
				},
				Egress: &as3ResourcePointer{
					BigIP: fmt.Sprintf("%v", cfg.Virtual.TCP.Server),
				},
			}
		}
	}

	// Attaching Profiles from Policy CRD
	for _, profile := range cfg.Virtual.Profiles {
		_, name := getPartitionAndName(profile.Name)
		switch profile.Context {
		case "udp":
			if !profile.BigIPProfile {
				svc.ProfileUDP = name
			} else {
				svc.ProfileUDP = &as3ResourcePointer{
					BigIP: fmt.Sprintf("%v", profile.Name),
				}
			}
		}
	}

	if cfg.Virtual.TranslateServerAddress == true {
		svc.TranslateServerAddress = cfg.Virtual.TranslateServerAddress
	}
	if cfg.Virtual.TranslateServerPort == true {
		svc.TranslateServerPort = cfg.Virtual.TranslateServerPort
	}
	if cfg.Virtual.Source != "" {
		svc.Source = cfg.Virtual.Source
	}
	virtualAddress, port := ap.extractVirtualAddressAndPort(cfg.Virtual.Destination)
	// verify that ip address and port exists.
	if virtualAddress != "" && port != 0 {
		if len(cfg.ServiceAddress) == 0 {
			va := append(svc.VirtualAddresses, virtualAddress)
			svc.VirtualAddresses = va
			svc.VirtualPort = port
		} else {
			//Attach Service Address
			serviceAddressName := ap.createServiceAddressDecl(cfg, virtualAddress, sharedApp)
			sa := &as3ResourcePointer{
				Use: serviceAddressName,
			}
			svc.VirtualAddresses = append(svc.VirtualAddresses, sa)
			svc.VirtualPort = port
		}
	}
	if cfg.Virtual.PoolName != "" {
		var poolPointer as3ResourcePointer
		ps := strings.Split(cfg.Virtual.PoolName, "/")
		poolPointer.Use = fmt.Sprintf("/%s/%s/%s",
			tenant,
			as3SharedApplication,
			ps[len(ps)-1],
		)
		svc.Pool = &poolPointer
	}
	ap.processCommonDecl(cfg, svc)
	sharedApp[cfg.Virtual.Name] = svc
}

// Process common declaration for VS and TS
func (ap *AS3Parser) processCommonDecl(cfg *ResourceConfig, svc *as3Service) {

	if cfg.Virtual.SNAT == "auto" || cfg.Virtual.SNAT == "none" || cfg.Virtual.SNAT == "self" {
		svc.SNAT = cfg.Virtual.SNAT
	} else {
		svc.SNAT = &as3ResourcePointer{
			BigIP: fmt.Sprintf("%v", cfg.Virtual.SNAT),
		}
	}
	// Enable connection mirroring
	if cfg.Virtual.ConnectionMirroring != "" {
		svc.Mirroring = cfg.Virtual.ConnectionMirroring
	}
	//Attach AllowVLANs
	if cfg.Virtual.AllowVLANs != nil {
		for _, vlan := range cfg.Virtual.AllowVLANs {
			vlans := as3ResourcePointer{BigIP: vlan}
			svc.AllowVLANs = append(svc.AllowVLANs, vlans)
		}
	}

	//Attach Firewall policy
	if cfg.Virtual.Firewall != "" {
		svc.Firewall = &as3ResourcePointer{
			BigIP: fmt.Sprintf("%v", cfg.Virtual.Firewall),
		}
	}

	//Attach ipIntelligence policy
	if cfg.Virtual.IpIntelligencePolicy != "" {
		svc.IpIntelligencePolicy = &as3ResourcePointer{
			BigIP: fmt.Sprintf("%v", cfg.Virtual.IpIntelligencePolicy),
		}
	}

	//Attach profile access policy
	// if perRequest policy is enabled, profile access policy should also be configured
	if cfg.Virtual.ProfileAccess != "" {
		svc.ProfileAccess = &as3ResourcePointer{
			BigIP: fmt.Sprintf("%v", cfg.Virtual.ProfileAccess),
		}

		//Attach per request policy
		if cfg.Virtual.PolicyPerRequestAccess != "" {
			svc.PolicyPerRequestAccess = &as3ResourcePointer{
				BigIP: fmt.Sprintf("%v", cfg.Virtual.PolicyPerRequestAccess),
			}
		}
	}

	//Attach logging profile
	if cfg.Virtual.LogProfiles != nil {
		for _, lp := range cfg.Virtual.LogProfiles {
			logProfile := as3ResourcePointer{BigIP: lp}
			svc.LogProfiles = append(svc.LogProfiles, logProfile)
		}
	}

	//Attach adapt profile
	if (cfg.Virtual.ProfileAdapt != ProfileAdapt{}) {
		if cfg.Virtual.ProfileAdapt.Request != "" {
			svc.ProfileRequestAdapt = &as3ResourcePointer{
				BigIP: fmt.Sprintf("%v", cfg.Virtual.ProfileAdapt.Request),
			}
		}
		if cfg.Virtual.ProfileAdapt.Response != "" {
			svc.ProfileResponseAdapt = &as3ResourcePointer{
				BigIP: fmt.Sprintf("%v", cfg.Virtual.ProfileAdapt.Response),
			}
		}
	}

	//Process iRules for crd
	ap.processIrulesForCRD(cfg, svc)
}

// getSortedCustomProfileKeys sorts customProfiles by names and returns secretKeys in that order
func (ap *AS3Parser) getSortedCustomProfileKeys(customProfiles map[SecretKey]CustomProfile) []SecretKey {
	keys := make([]SecretKey, len(customProfiles))
	i := 0
	for key := range customProfiles {
		keys[i] = key
		i++
	}
	sort.Slice(keys, func(i, j int) bool {
		return customProfiles[keys[i]].Name < customProfiles[keys[j]].Name
	})
	return keys
}

func updateVirtualToHTTPS(v *as3Service) {
	v.Class = "Service_HTTPS"
	redirect80 := false
	v.Redirect80 = &redirect80
}

func DeepEqualJSON(decl1, decl2 as3Declaration) bool {
	if decl1 == "" && decl2 == "" {
		return true
	}
	var o1, o2 interface{}

	err := json.Unmarshal([]byte(decl1), &o1)
	if err != nil {
		return false
	}

	err = json.Unmarshal([]byte(decl2), &o2)
	if err != nil {
		return false
	}

	return reflect.DeepEqual(o1, o2)
}
