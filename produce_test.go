package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

func TestHashCode(t *testing.T) {

	data := []struct {
		in       string
		expected int32
	}{
		{
			in:       "",
			expected: 0,
		},
		{
			in:       "a",
			expected: 97,
		},
		{
			in:       "b",
			expected: 98,
		},
		{
			in:       "⌘",
			expected: 8984,
		},
		{
			in:       "😼", //non-bmp character, 4bytes in utf16
			expected: 1772959,
		},
		{
			in:       "hashCode",
			expected: 147696667,
		},
		{
			in:       "c03a3475-3ed6-4ed1-8ae5-1c432da43e73",
			expected: 1116730239,
		},
		{
			in:       "random",
			expected: -938285885,
		},
	}

	for _, d := range data {
		actual := hashCode(d.in)
		if actual != d.expected {
			t.Errorf("expected %v but found %v\n", d.expected, actual)
		}
	}
}

func TestHashCodePartition(t *testing.T) {

	data := []struct {
		key        string
		partitions int32
		expected   int32
	}{
		{
			key:        "",
			partitions: 0,
			expected:   -1,
		},
		{
			key:        "",
			partitions: 1,
			expected:   0,
		},
		{
			key:        "super-duper-key",
			partitions: 1,
			expected:   0,
		},
		{
			key:        "",
			partitions: 1,
			expected:   0,
		},
		{
			key:        "",
			partitions: 2,
			expected:   0,
		},
		{
			key:        "a",
			partitions: 2,
			expected:   1,
		},
		{
			key:        "b",
			partitions: 2,
			expected:   0,
		},
		{
			key:        "random",
			partitions: 2,
			expected:   1,
		},
		{
			key:        "random",
			partitions: 5,
			expected:   0,
		},
	}

	for _, d := range data {
		actual := hashCodePartition(d.key, d.partitions)
		if actual != d.expected {
			t.Errorf("expected %v but found %v for key %#v and %v partitions\n", d.expected, actual, d.key, d.partitions)
		}
	}
}

func TestProduceParseArgs(t *testing.T) {
	configBefore := config
	defer func() {
		config = configBefore
	}()

	expectedTopic := "test-topic"
	givenBroker := "hans:9092"
	expectedBrokers := []string{givenBroker}

	config.produce.args.topic = ""
	config.produce.args.brokers = ""
	os.Setenv("KT_TOPIC", expectedTopic)
	os.Setenv("KT_BROKERS", givenBroker)

	produceParseArgs()
	if config.produce.topic != expectedTopic ||
		!reflect.DeepEqual(config.produce.brokers, expectedBrokers) {
		t.Errorf(
			"Expected topic %v and brokers %v from env vars, got topic %v and brokers %v.",
			expectedTopic,
			expectedBrokers,
			config.produce.topic,
			config.produce.brokers,
		)
		return
	}

	// default brokers to localhost:9092
	os.Setenv("KT_TOPIC", "")
	os.Setenv("KT_BROKERS", "")
	config.produce.args.topic = expectedTopic
	config.produce.args.brokers = ""
	expectedBrokers = []string{"localhost:9092"}

	produceParseArgs()
	if config.produce.topic != expectedTopic ||
		!reflect.DeepEqual(config.produce.brokers, expectedBrokers) {
		t.Errorf(
			"Expected topic %v and brokers %v from env vars, got topic %v and brokers %v.",
			expectedTopic,
			expectedBrokers,
			config.produce.topic,
			config.produce.brokers,
		)
		return
	}

	// command line arg wins
	os.Setenv("KT_TOPIC", "BLUBB")
	os.Setenv("KT_BROKERS", "BLABB")
	config.produce.args.topic = expectedTopic
	config.produce.args.brokers = givenBroker
	expectedBrokers = []string{givenBroker}

	produceParseArgs()
	if config.produce.topic != expectedTopic ||
		!reflect.DeepEqual(config.produce.brokers, expectedBrokers) {
		t.Errorf(
			"Expected topic %v and brokers %v from env vars, got topic %v and brokers %v.",
			expectedTopic,
			expectedBrokers,
			config.produce.topic,
			config.produce.brokers,
		)
		return
	}
}

func newMessage(key, value string, partition int32) message {
	var k *string
	if key != "" {
		k = &key
	}

	var v *string
	if value != "" {
		v = &value
	}

	return message{
		Key:       k,
		Value:     v,
		Partition: &partition,
	}
}

func TestDeserializeLines(t *testing.T) {
	config.produce.partitioner = "hashCode"
	data := []struct {
		in             string
		literal        bool
		partition      int32
		partitionCount int32
		expected       message
	}{
		{
			in:             "",
			literal:        false,
			partitionCount: 1,
			expected:       newMessage("", "", 0),
		},
		{
			in:             `{"key":"hans","value":"123"}`,
			literal:        false,
			partitionCount: 4,
			expected:       newMessage("hans", "123", hashCodePartition("hans", 4)),
		},
		{
			in:             `{"key":"hans","value":"123","partition":1}`,
			literal:        false,
			partitionCount: 3,
			expected:       newMessage("hans", "123", 1),
		},
		{
			in:             `{"other":"json","values":"avail"}`,
			literal:        true,
			partition:      2,
			partitionCount: 4,
			expected:       newMessage("", `{"other":"json","values":"avail"}`, 2),
		},
		{
			in:             `so lange schon`,
			literal:        false,
			partitionCount: 3,
			expected:       newMessage("", "so lange schon", 0),
		},
	}

	for _, d := range data {
		in := make(chan string, 1)
		out := make(chan message)
		config.produce.literal = d.literal
		config.produce.partition = d.partition
		go deserializeLines(in, out, d.partitionCount)
		in <- d.in

		select {
		case <-time.After(50 * time.Millisecond):
			t.Errorf("did not receive output in time")
		case actual := <-out:
			if !(reflect.DeepEqual(d.expected, actual)) {
				t.Errorf(spew.Sprintf("\nexpected %#v\nactual   %#v", d.expected, actual))
			}
		}
	}
}
