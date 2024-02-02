package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	imap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

type imapControler struct {
	client          *client.Client
	inbox           string
	mbox            *imap.MailboxStatus
	lastProccesedId uint32
	fs              *firestore.Client
}

func (ic *imapControler) Init(Server, Username, Password string) error {
	c, err := client.DialTLS(Server, nil)
	if err != nil {
		return err
	}
	ic.client = c
	log.Printf("[DEBUG] imap: connected")
	err = ic.client.Login(Username, Password)
	if err != nil {
		return err
	}
	if ic.inbox == "" {
		ic.inbox = "inbox"
	}
	mbox, err := ic.client.Select(ic.inbox, false)
	if err != nil {
		return err
	}
	ic.mbox = mbox
	log.Printf("[DEBUG] imap: logged in")
	return nil
}

func (ic *imapControler) Close() error {
	log.Printf("[DEBUG] imap: logout")
	if err := ic.client.Logout(); err != nil {
		log.Printf("[ERROR] imap: logout error %v", err)
		return err
	}
	return nil
}

func (ic *imapControler) ProcMessages() error {
	//ds := fs.Collection("dmarc").Document("imap").Get(ctx)
	var ds *int
	ds = nil
	if ds == nil {
		ic.lastProccesedId = 1
	} else {
		ic.lastProccesedId = 2
	}
	lastMessage := ic.mbox.Messages
	if ic.lastProccesedId == lastMessage {
		return nil
	}
	seqSet := new(imap.SeqSet)
	seqSet.AddRange(ic.lastProccesedId, lastMessage)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{section.FetchItem()}
	messages := make(chan *imap.Message, 10)
	go func() {
		if err := ic.client.Fetch(seqSet, items, messages); err != nil {
			log.Printf("[ERROR] imap: fetch error %v", err)
		}
	}()
	for msg := range messages {
		br := msg.GetBody(section)
		if br == nil {
			return fmt.Errorf("server didn't return message body")
		}

		// create a new mail reader
		mr, err := mail.CreateReader(br)
		if err != nil {
			return err
		}

		// process each message's part
		isSuccess := false
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				log.Printf("[ERROR] imap: can't read next part: %v, skip", err)
				break
			}

			switch h := p.Header.(type) {
			case mail.AttachmentHeader:
				// this is an attachment
				filename, err := h.Filename()
				if err != nil {
					log.Printf("[ERROR] imap: %v, skip", err)
					continue
				}
				log.Printf("[INFO] imap: found attachment: %v", filename)

				report, err := readParse(p.Body, filename, false)
				if err != nil {
					log.Printf("[ERROR] parsing: %v, skip", err)
					continue
				}
				fmt.Printf("%+v\n", report)
				isSuccess = true
			}
		}
		fmt.Printf("Success: %v\n", isSuccess)
	}
	ic.lastProccesedId = lastMessage
	return nil
}

func main() {
	var (
		flagVersion bool
		flagConfig  string
	)

	flag.BoolVar(&flagVersion, "version", false, "show version and exit")
	flag.StringVar(&flagConfig, "config", "./config.yaml", "Path to config file")
	flag.Parse()

	if flagVersion {
		fmt.Printf("Version: %v\n", version)
		os.Exit(0)
	}

	cfg, err := loadConfig(flagConfig)
	if err != nil {
		log.Fatalf("[ERROR] loadConfig: %v", err)
	}

	setupLog(cfg.LogDebug, cfg.LogDatetime)

	imapCtrl := new(imapControler)
	err = imapCtrl.Init(cfg.Input.IMAP.Server, cfg.Input.IMAP.Username, cfg.Input.IMAP.Password)
	if err != nil {
		log.Fatalf("[ERROR] imap: %v", err)
	}
	defer imapCtrl.Close()
	imapCtrl.ProcMessages()
	fmt.Print("End")
}
