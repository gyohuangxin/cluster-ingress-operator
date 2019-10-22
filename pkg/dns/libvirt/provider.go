package libvirt

import (
	"encoding/xml"
	"net"
	"strings"

	libvirt "github.com/libvirt/libvirt-go"

	configv1 "github.com/openshift/api/config/v1"
	iov1 "github.com/openshift/cluster-ingress-operator/pkg/api/v1"

	logf "github.com/openshift/cluster-ingress-operator/pkg/log"
)

var (
	// _ dns.Provider = &Provider{}
	log = logf.Logger.WithName("entrypoint")
)

type Provider struct {
	// config is required input.
	config Config
}

type Config struct {
	Cluster string
	Domain  string
	Url     string
}
type Host struct {
	XMLName   xml.Name `xml:"host"`
	Ip        string   `xml:"ip,attr"`
	DnsRecord string   `xml:"hostname"`
}

type action = libvirt.NetworkUpdateCommand

const (
	upsertAction action = 3
	deleteAction action = 2
)

// New creates (but does not start) a new operator from configuration.
func New(config Config) (*Provider, error) {
	provider := &Provider{
		config: config,
	}
	return provider, nil
}

func (p *Provider) Ensure(record *iov1.DNSRecord, zone configv1.DNSZone) error {
	return p.change(record, zone, upsertAction)
}

func (p *Provider) Delete(record *iov1.DNSRecord, zone configv1.DNSZone) error {
	return p.change(record, zone, deleteAction)
}

// Change methods will perform an action on a record.
func (p *Provider) change(record *iov1.DNSRecord, zone configv1.DNSZone, action action) error {
	// Create a new connections to qemu
	conn, err := libvirt.NewConnect(p.config.Url)
	if err != nil {
		log.Error(err, "failed to connect qemu")
	}

	// List all networks
	networks, err := conn.ListAllNetworks(2)
	if err != nil {
		log.Error(err, "failed to get networks")
	}

	// Lookup app domain's ip
	ips, err := net.LookupIP(p.config.Domain)
	if err != nil || len(ips) == 0 {
		log.Error(err, "failed to lookup app domain's IPs")
	}
	log.Info("Domain nslookup", "IPs", ips)

	// Find the correct network and update a DNS record
	for _, network := range networks {
		name, err := network.GetName()
		if err != nil {
			log.Error(err, "failed to get network name")
		}
		if strings.Index(name, p.config.Cluster) == -1 {
			log.Info("find the network but not in use", "network", name)
			continue
		}
		log.Info("found network in use", "network", name)
		for _, ip := range ips {
			// Generate a XML for DNS host
			v := &Host{Ip: ip.String(), DnsRecord: record.Spec.DNSName}
			output, err := xml.MarshalIndent(v, "  ", "    ")
			if err != nil {
				log.Error(err, "failed to generate a network XML")
			}
			log.Info("DNS record is updating", "XML", string(output))
			// Update the network
			log.Info("DNS record is updating", "action", action)
			err = network.Update(action, 10, -1, string(output), 0)
			if err != nil {
				log.Error(err, "failed to update network")
			}
		}
		if err != nil {
			log.Error(err, "failed to get networks")
		}
		network.Free()
	}
	conn.Close()
	return nil
}
