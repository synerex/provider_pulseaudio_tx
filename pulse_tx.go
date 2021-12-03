package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/jfreymuth/pulse" // for pulseaudio recording

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/proto"

	storage "github.com/synerex/proto_storage"
	pb "github.com/synerex/synerex_api"
	pbase "github.com/synerex/synerex_proto"
	sxutil "github.com/synerex/synerex_sxutil"
)

var (
	nodesrv      = flag.String("nodesrv", "127.0.0.1:9990", "Node ID Server")
	local        = flag.String("local", "", "Specify Local Synerex Server")
	fragment     = flag.Int("fragment", 2048, "Audio capture fragment size")
	rate         = flag.Int("rate", 44100, "Audio capture sampling rate")
	verbose      = flag.Bool("verbose", false, "Verbose audio information")
	nosend       = flag.Bool("nosend", false, "Not sending audio information")
	threshold    = flag.Float64("threshold", 0.0, "Audio threshold mode (float)")
	lastUnixNano int64
	sxclient     *sxutil.SXServiceClient
)

func init() {

}

func SxSend(p []float32) (int, error) {
	now := time.Now().UnixNano()
	//	return len(p), binary.Write(f.out, binary.LittleEndian, p)
	var avg float64 = 0.0
	for i := 0; i < len(p); i++ {
		avg += math.Abs(float64(p[i]))
	}
	avg = avg / float64(len(p))
	v := uint8(math.Log2(avg * 10000))
	if *verbose {
		if *threshold != 0.0 && *threshold < avg {
			fmt.Printf("Audio Power %d %f(Over Threshold), datalen:%d timeDiff(nsec):%d \n", v, avg, len(p), now-lastUnixNano)
		} else {
			fmt.Printf("Audio Power %d %f, datalen:%d timeDiff(nsec):%d \n", v, avg, len(p), now-lastUnixNano)
		}
	}
	if *nosend == false {
		if *threshold == 0.0 || *threshold < avg {
			// send Synerex
			// currently use Storage channel. but it should be Media Channel

			ts, _ := ptypes.TimestampProto(time.Now().Add(9 * time.Hour)) // for JST conversion
			bytes := []byte{v}

			media := &storage.Record{
				BucketName: "AudioVolume",
				ObjectName: ptypes.TimestampString(ts),
				Record:     bytes,
			}

			out, _ := proto.Marshal(media)

			cont := pb.Content{Entity: out}

			smo := sxutil.SupplyOpts{
				Name:  "AudioVolume",
				Cdata: &cont,
			}
			_, nerr := sxclient.NotifySupply(&smo)
			if nerr != nil {
				log.Printf("Error !! for Notify")
			}
		}
	}

	lastUnixNano = now
	return len(p), nil
}

func startCaptureAudio() {
	opts := []pulse.ClientOption{
		pulse.ClientApplicationName("SxProviderTX"),
	}
	c, err := pulse.NewClient(opts...)
	if err != nil {
		fmt.Println(err)
		return
	}

	ropts := []pulse.RecordOption{
		pulse.RecordSampleRate(*rate),                     // default 44100
		pulse.RecordBufferFragmentSize(uint32(*fragment)), // default 2048
	}

	stream, err := c.NewRecord(pulse.Float32Writer(SxSend), ropts...)
	if err != nil {
		fmt.Println(err)
		return
	}

	stream.Start()

	// no grace close..( c.close, stream.stop)
}

func main() {
	log.Printf("PulseAudio Transfer Provider(%s) built %s sha1 %s", sxutil.GitVer, sxutil.BuildTime, sxutil.Sha1Ver)
	flag.Parse()
	go sxutil.HandleSigInt()
	sxutil.RegisterDeferFunction(sxutil.UnRegisterNode)

	//	channelTypes := []uint32{pbase.MEDIA_SVC}
	channelTypes := []uint32{pbase.STORAGE_SERVICE}
	srv, rerr := sxutil.RegisterNode(*nodesrv, "PulseAudio", channelTypes, nil)

	if rerr != nil {
		log.Fatal("Can't register node:", rerr)
	}
	if *local != "" { // quick hack for AWS local network
		srv = *local
	}
	log.Printf("Connecting SynerexServer at [%s]", srv)

	//	wg := sync.WaitGroup{} // for syncing other goroutines

	gclient := sxutil.GrpcConnectServer(srv)

	if gclient == nil {
		log.Fatal("Can't connect Synerex Server")
	} else {
		log.Print("Connecting SynerexServer")
	}

	argJson := fmt.Sprintf("{PulseAudio:Rate(%d),Frag(%d)}", *rate, *fragment)

	//	sxclient = sxutil.NewSXServiceClient(gclient, pbase.MEDIA_SVC, argJson)
	sxclient = sxutil.NewSXServiceClient(gclient, pbase.STORAGE_SERVICE, argJson)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	startCaptureAudio()

	wg.Wait()
}
