package node

// MessagingFlavor classifica o tipo de canal de mensageria.
type MessagingFlavor string

const (
	MessagingQueue  MessagingFlavor = "queue"  // SQS, Pub/Sub queue
	MessagingTopic  MessagingFlavor = "topic"  // SNS, Pub/Sub topic
	MessagingStream MessagingFlavor = "stream" // Kinesis, Pub/Sub Lite
	MessagingBroker MessagingFlavor = "broker" // MSK, MQ, AMQ
)

// Messaging representa recursos de mensageria/eventos.
type Messaging struct {
	Base
	ProviderID   ProviderID        `json:"provider_id"`
	Account      URN               `json:"account_urn"`
	Region       URN               `json:"region_urn"`
	ExtID        string            `json:"external_id"`
	Flavor       MessagingFlavor   `json:"flavor"`
	Protocol     string            `json:"protocol,omitempty"` // "kafka", "amqp", "sqs"
	RetentionSec uint32            `json:"retention_sec,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	SpecRaw      map[string]any    `json:"spec_raw,omitempty"`
}

// Resource interface implementation.
func (m Messaging) Provider() ProviderID          { return m.ProviderID }
func (m Messaging) AccountURN() URN               { return m.Account }
func (m Messaging) RegionURN() URN                { return m.Region }
func (m Messaging) ExternalID() string            { return m.ExtID }
func (m Messaging) NativeTags() map[string]string { return m.Tags }
func (m Messaging) Spec() map[string]any          { return m.SpecRaw }
