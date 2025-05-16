// /!\ This version is hosted directly on 192.168.2.22. /!\

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// General settings
var relayUrl string = "ws://192.168.2.22:7777"
var defaultNpub string = "npub1zc4r69x9nxg7h0qcs705k6c5yt7xaydtjrlsthk99vmgg2t2xgssd7mdde" // npub used as a starting point when none is specified

// Algorithm parameters
var minimumTrustScore float64 = 0.000002 // originally 10^-6 (arbitrary) (used for follows, mutes)
var followEnable bool = true // use follows in the calculation of trust scores? (yes, you do want to keep this enabled)
var followWeight float64 = 1.0 // with k the nb of people I follow; people I follow have a score of followWeight/k; an account followed by one person i follow will have a score of followWeight**2/(k*k2)
var followTTL int = 2 // 2 ok, recommended ; 3 only locally ; don't try more // 1 = consider only my followees, 2 = them and the people they follow. Keep it low to prevent weird values due to potential loops
var muteEnable bool = true // use mutes in the calculation of trust scores?
var reportsEnable bool = true // use reports in the calculation of trust scores?
var untrustedReportsWeight float64 = 0.000001 // if user A is reported by N untrusted users, A will get a score of -N*untrustedReportsWeight
var repostszapsEnable bool = true // use reposts and zaps to calculate trust (interactive mode only)
var repostWeight float64 = 0.001 // the trust given per repost from a trusted user
//var zapWeight float64 = minimumTrustScore / 1000 // the trust given sat zapped from a trusted user

var benchmarkPubkeys  = []string{
	"npub1sn0wdenkukak0d9dfczzeacvhkrgz92ak56egt7vdgzn8pv2wfqqhrjdv9",
	"npub1xtscya34g58tk0z605fvr788k263gsu6cy9x0mhnm87echrgufzsevkk5s",
	"npub1wmr34t36fy03m8hvgl96zl3znndyzyaqhwmwdtshwmtkg03fetaqhjg240",
	"npub1tjkc9jycaenqzdc3j3wkslmaj4ylv3dqzxzx0khz7h38f3vc6mls4ys9w3",
	"npub14tq8m9ggnnn2muytj9tdg0q6f26ef3snpd7ukyhvrxgq33vpnghs8shy62",
	"npub1psjwxg6j9re7u5mf6j52gm4f9u50r2pexm0xrtrtgqpagq6y6s3qhfa504",
	"npub1zfwrn5gnl9gml2lnr8kmarqdsd9yle7l5rvv84prz0ghjq8nerrs73sl0t",
	"npub1y0gvydcezdjkg6r3gnudhx8ttjzgvsn5sw0um7ucrdrphapwzn5qy68zn4",
	"npub14rj7arqrd5dwu927p4wm4m85lwma8c8ccupxxrkzy6hpj6glcw9qmse63g",
	"npub16g0r66400k8j3d5c7v9ucjr6vd4xg3nhzknrxvnvh72gg2w39kksfmqpk7",
	"npub14ah9dgcedd4xza3dzpvg3urjq8qy2jr6xecz3w4y006kqjcygplq9xgrvk",
	"npub19qharf5037zxxp8dgws7ukcxqjvxt9vq3rf3gqye94ga85jmu2dsvyy3ww",
	"npub1guux40z0lnx7zmwcts7qshnf2emx4rsd2e5sncj23h75s7meg7xsj9vsy9",
	"npub1wgdxae2recdf3h4ss9w9cdn7dse4hvfckjh2pw3kwxwd5x5e8kdqqaxhkm",
	"npub14jmwlyhcl7ac93p96y67rw8vrz96p46d7f6venpscyfsczahqx2s6xxwm3",
	"npub1sckm8y5d4a6p73zjhu7thhsjmzfetyuvdx2ch0lr3h8wwnx4z97qujqktv",
	"npub1tkd09vxhv687dgevp50usmkrlkdquc8nceq6qusumf93ggfytqaqxzujpk",
	"npub1e537pwwunau3wxzh8ed6k6wevly53afq6km3g723kcesgkd4fdds2ahq3m",
	"npub1049yaplj3c8rtqw542frf9xlkka59z4a4ssdk72kpc4aajznhw5q8ra7md",
	"npub15d9enu3v0yxyud4jk0pvxk3kmvrzymjpc6f0eq4ck44vr32qck7smrxq6k",
}

var verbose bool


func npub2hexpub(npub string) string {
	if _, hexpub, err := nip19.Decode(npub); err == nil {
		return hexpub.(string)
	} else {
		fmt.Println("ERROR: The program encountered an invalid npub! Using default npub instead.")
		return npub2hexpub(defaultNpub)
	}
}

func hexpub2npub(hexpub string) string {
	if npub, err := nip19.EncodePublicKey(hexpub); err == nil {
		return npub
	} else {
		panic("The program has encountered an invalid hexadecimal pubkey!")
	}
}

func getDisplayName(hexpub string) string {
	if verbose {
		fmt.Println("DEBUG: Getting name of user with npub:", hexpub2npub(hexpub)) //debug
		//return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

	// connect to relay
	url := relayUrl
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		return hexpub2npub(hexpub)
		//panic(err)
	}

	// create filters
	var filters nostr.Filters

	filters = []nostr.Filter{{
		Kinds: []int{nostr.KindProfileMetadata},
		Authors: []string{hexpub},
		Limit: 1,
	}}

	// create a subscription and submit to relay
	// results will be returned on the sub.Events channel
	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		return hexpub2npub(hexpub)
		//panic(err)
	}

	go func() { // goroutine
		<-sub.EndOfStoredEvents // channel that closes once the relay is done sending past events
		cancel()
	}()
	ev := <-sub.Events
	if ev == nil {
		return hexpub2npub(hexpub)
	}

	var profileMetadata map[string]interface{}
	if err := json.Unmarshal([]byte(ev.Content), &profileMetadata); err != nil {
		//panic(err)
		fmt.Println("ERROR: Lost connection to relay. Request skipped.")
		return hexpub2npub(hexpub)
	}

	var returnedName string
	if profileMetadata["display_name"] != nil && profileMetadata["display_name"] != "" {
		returnedName = profileMetadata["display_name"].(string)
	} else if profileMetadata["name"] != nil && profileMetadata["name"] != "" {
		returnedName = profileMetadata["name"].(string)
	} else {
		returnedName = hexpub2npub(hexpub)
	}

	return returnedName
}

func getFollows(hexpub string) []string { // returns a slice of hexpubs
	if verbose {
		fmt.Println("DEBUG: Getting follows for", hexpub) //debug
	}
	follows := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	// connect to relay
	url := relayUrl
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		panic(err)
	}

	// create filters
	var filters nostr.Filters

	filters = []nostr.Filter{{
		Kinds: []int{nostr.KindFollowList},
		Authors: []string{hexpub},
		Limit: 1,
	}}

	// create a subscription and submit to relay
	// results will be returned on the sub.Events channel
	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		panic(err)
	}

	go func() { // goroutine
		<-sub.EndOfStoredEvents // channel that closes once the relay is done sending past events
		cancel()
	}()
	ev := <-sub.Events
	if ev == nil {
		return follows
	}

	for _, tag := range ev.Tags {
		if tag[0] == "p" {
			follows = append(follows, tag[1])
		}
	}

	return follows
}

func trustFollows(trustRatings map[string]float64, hexpub string, TTL int) { // no need to pass a pointer for trustRatings, as maps are reference types by default
	// recursion base case
	if !followEnable || TTL < 1 {
		return
	}

	// fetch people the current pubkey follows and update their trust rating
	follows := getFollows(hexpub)
	x := followWeight
	k := float64(len(follows))

	for _, followed := range follows {
		rating := trustRatings[followed] + trustRatings[hexpub] * x / k
		if rating >= minimumTrustScore {
			trustRatings[followed] = min(rating, 1)
		}
	}

	// update trust ratings of the next level follows (recursion) (could be possible to merge both loops into a single loop, but I find it more explicit like this)
	for _, followed := range follows {
		trustFollows(trustRatings, followed, TTL-1)
	}
}

func getMutes(hexpub string) []string { // returns a slice of hexpubs
	// for future refactoring: except for the kind number, this function should be pretty much identical to getFollows
	// actually, instead of sending tons of queries, the program could be optimized by sending one query with all the pubkeys at once, and then processing things locally
	if verbose {
		fmt.Println("DEBUG: Getting mutes for", hexpub) //debug
	}
	mutes := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	// connect to relay
	url := relayUrl
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		//panic(err)
		fmt.Println("ERROR: Lost connection to relay. Request skipped.")
	}

	// create filters
	var filters nostr.Filters

	filters = []nostr.Filter{{
		Kinds: []int{nostr.KindMuteList},
		Authors: []string{hexpub},
		Limit: 1,
	}}

	// create a subscription and submit to relay
	// results will be returned on the sub.Events channel
	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		panic(err)
	}

	go func() { // goroutine
		<-sub.EndOfStoredEvents // channel that closes once the relay is done sending past events
		cancel()
	}()
	ev := <-sub.Events
	if ev == nil {
		return mutes
	}

	for _, tag := range ev.Tags {
		if tag[0] == "p" {
			mutes = append(mutes, tag[1])
		}
	}

	return mutes
}

func distrustMutes(trustRatings map[string]float64) {
	if !muteEnable {
		return
	}
	fmt.Println("The program is going through mutes. This can take a while.")
	mutedRatings := make(map[string]float64) // this will be later merged with trustRatings

	// for every trusted pubkey, look up who they have muted...
	for pub, score := range trustRatings {
		if score > 0 {
			mutes := getMutes(pub)
			k := float64(len(mutes))

			// ...and store their negative trust
			for _, muted := range mutes {
				if score / k >= minimumTrustScore {
					mutedRatings[muted] = - score / k
				}
			}
		}
	}

	if verbose {
		fmt.Println("DEBUG: len(trustRatings) =", len(trustRatings)) //debug
		fmt.Println("DEBUG: len(mutedRatings) =", len(mutedRatings)) //debug
	}

	// then merge the trust lists
	for pub, score := range mutedRatings {
		trustRatings[pub] += score
	}
}

func getReportedBy(hexpub string) []string { // returns a slice of hexpubs the given account has been reported by
	if verbose {
		fmt.Println("DEBUG: Checking if user with public key", hexpub, "has been reported") //debug
	}
	reportedBy := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	// connect to relay
	url := relayUrl
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		panic(err)
		fmt.Println("ERROR: Lost connection to relay. Request skipped.")
	}

	// create filters
	var filters nostr.Filters

	t := make(map[string][]string)
	// making a "p" tag for the above public key.
	// this filters for messages tagged with the user.
	t["p"] = []string{hexpub}

	filters = []nostr.Filter{{
		Kinds: []int{nostr.KindReporting},
		Tags:  t,
		Limit: 1000,
	}}

	// create a subscription and submit to relay
	// results will be returned on the sub.Events channel
	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		panic(err)
	}

	go func() { // goroutine
		<-sub.EndOfStoredEvents // channel that closes once the relay is done sending past events
		cancel()
	}()
	for ev := range sub.Events {
		if verbose {
			fmt.Println("DEBUG: Pubkey that reported", hexpub, "is:", ev.PubKey) //debug
		}
		if !slices.Contains(reportedBy, ev.PubKey) {
			reportedBy = append(reportedBy, ev.PubKey)
		}
	}

	return reportedBy
}

func getReports(hexpub string) []string { // returns a slice of reports sent by the given pubkey
	if verbose {
		fmt.Println("DEBUG: Getting reports from", hexpub) //debug
	}
	reports := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	// connect to relay
	url := relayUrl
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		//panic(err)
		fmt.Println("ERROR: Lost connection to relay. Request skipped.")
	}

	// create filters
	var filters nostr.Filters

	filters = []nostr.Filter{{
		Kinds: []int{nostr.KindReporting},
		Authors: []string{hexpub},
		Limit: 1000,
	}}

	// create a subscription and submit to relay
	// results will be returned on the sub.Events channel
	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		panic(err)
	}

	go func() { // goroutine
		<-sub.EndOfStoredEvents // channel that closes once the relay is done sending past events
		cancel()
	}()
	for ev := range sub.Events {
		for _, tag := range ev.Tags {
			if tag[0] == "p" {
				if !slices.Contains(reports, tag[1]) {
					if verbose {
						fmt.Println("DEBUG:", hexpub, "reported", tag[1]) //debug
					}
					reports = append(reports, tag[1])
				}
			}
		}	
	}

	return reports
}

func distrustReported(trustRatings map[string]float64) {
	if !reportsEnable {
		return
	}
	fmt.Println("The program is going through reports. This can take a while.")
	// Here we do two things:
	// 1. reports sent from trusted accounts give the opposite trust amount to reported account
	// 2. [moved out; only in interactive mode] users in trustRatings that have reports from other people get a constant penalty

	reportedRatings := make(map[string]float64) // this will be later merged with trustRatings

	for pub, score := range trustRatings {
		// 1. for every trusted pubkey, look up who they have reported...
		if score > 0.00005 {
			reports := getReports(pub)
			// k := float64(len(reports))

			// ...and store their negative trust
			for _, reported := range reports {
				// if score / k >= minimumTrustScore {
					reportedRatings[reported] = - score // / k
				// }
			}
		}
	}

	if verbose {
		fmt.Println("DEBUG: len(trustRatings) =", len(trustRatings)) //debug
		fmt.Println("DEBUG: len(reportedRatings) =", len(reportedRatings)) //debug
	}

	// then merge the trust lists
	for pub, score := range reportedRatings {
		trustRatings[pub] += score
	}
}

func distrustReportedByUntrusted(trustRatings map[string]float64) { // interactive mode only
	// 2. from distrustReported ; used only in interactive mode for performance reasons
	if !reportsEnable {
		return
	}

	reportedRatings := make(map[string]float64) // this will be later merged with trustRatings

	for pub, _ := range trustRatings {
		for _, reporter := range getReportedBy(pub) {
			_, exists := trustRatings[reporter]
			if !exists { // if reporter doesn't have a trust rating, use untrustedReportsWeight
				reportedRatings[pub] = - untrustedReportsWeight
			}
		}
	}

	if verbose {
		fmt.Println("DEBUG: len(interactiveRating) =", len(trustRatings)) //debug
		fmt.Println("DEBUG: len(reportedRatings) =", len(reportedRatings)) //debug
	}

	// then merge the trust lists
	for pub, score := range reportedRatings {
		trustRatings[pub] += score
	}
}

func getReposts(hexpub string) []string { // returns a slice of reposted pubkeys (duplicates possible)
	if verbose {
		fmt.Println("DEBUG: Getting reposts from", hexpub) //debug
	}
	reposts := make([]string, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	// connect to relay
	url := relayUrl
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		//panic(err)
		fmt.Println("ERROR: Lost connection to relay. Request skipped.")
	}

	// create filters
	var filters nostr.Filters

	filters = []nostr.Filter{{
		Kinds: []int{nostr.KindRepost, nostr.KindGenericRepost},
		Authors: []string{hexpub},
		Limit: 2000,
	}}

	// create a subscription and submit to relay
	// results will be returned on the sub.Events channel
	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		panic(err)
	}

	go func() { // goroutine
		<-sub.EndOfStoredEvents // channel that closes once the relay is done sending past events
		cancel()
	}()
	for ev := range sub.Events {
		for _, tag := range ev.Tags {
			if tag[0] == "p" {
				reposts = append(reposts, tag[1])
			}
		}	
	}

	return reposts
}

func trustReposts(trustRatings map[string]float64) {
	if !repostszapsEnable {
		return
	}
	fmt.Println("The program is going through reposts. This can take a while.")
	repostsRatings := make(map[string]float64) // this will be later merged with trustRatings

	// for every trusted pubkey, look up who they have reposted...
	for pub, score := range trustRatings {
		if score > 100 * minimumTrustScore {
			reposts := getReposts(pub)

			// ...and store their trust
			for _, reposted := range reposts {
				repostsRatings[reposted] = score * repostWeight
			}
		}
	}

	if verbose {
		fmt.Println("DEBUG: len(trustRatings) =", len(trustRatings)) //debug
		fmt.Println("DEBUG: len(repostsRatings) =", len(repostsRatings)) //debug
	}

	// then merge the trust lists
	for pub, score := range repostsRatings {
		trustRatings[pub] += score
	}
	
}

func printRatings(trustRatings map[string]float64) {
	// convert map to slice
	type Rating struct {
		Npub string
		TrustScore float64
		DisplayName string
	}

	orderedRatings := make([]Rating, 0, len(trustRatings))
	for pub, score := range trustRatings {
		orderedRatings = append(orderedRatings, Rating{hexpub2npub(pub), score, getDisplayName(pub)})
	}

	// sort the slice
	sort.Slice(orderedRatings, func(i, j int) bool { return orderedRatings[i].TrustScore > orderedRatings[j].TrustScore })

	// print the sorted scores
	fmt.Println("All trust ratings, sorted from most trusted to least trusted above threshold", minimumTrustScore, ":")
	for _, rating := range orderedRatings {
		fmt.Printf("%v  %.10f  %v\n", rating.Npub, rating.TrustScore, rating.DisplayName)
	}
}

func main() {
	// CLI parameters declaration & parsing
	var interactive bool
	flag.BoolVar(&interactive, "i", true, "Don't exit once ratings are displayed. Instead, switch to interactive mode. Enabled by default.")
	var npub string
	flag.StringVar(&npub, "npub", defaultNpub, "The npub used as a starting point for trust calculations.")
	var testArg bool
	flag.BoolVar(&verbose, "v", false, "Verbose mode: print debug information.")
	// TODO: check the implementation of -benchmark parameter
	var benchmark bool
	flag.BoolVar(&benchmark, "benchmark", false, "Benchmark mode: the program works as usual but display the scores of the benchmark keys separately at the end of the output.")

	flag.Parse()

	hexpub := npub2hexpub(npub)

	// Initializing the map
	trustRatings := make(map[string]float64)
	trustRatings[hexpub] = 1

	// Apply trust ratings for 'follow' relationship
	trustFollows(trustRatings, hexpub, followTTL)

	// Apply trust ratings for 'mute' relationship
	distrustMutes(trustRatings)

	// Apply trust ratings for reports
	distrustReported(trustRatings)

	// Apply trust ratings for reposts
	trustReposts(trustRatings)

	// trim values under minimumTrustScore
	// for key, value := range trustRatings {
	// 	if 0 <= value && value < minimumTrustScore {
	// 		delete(trustRatings, key)
	// 	}
	// }
	
	printRatings(trustRatings)

	// if -benchmark specified, display benchmark scores
	if benchmark {
		fmt.Println("Applying untrusted reports and displaying scores for benchmark pubkeys.")

		// create a map with only the ratings we're interested in
		benchmarkRatings := make(map[string]float64)
		for _, npub := range benchmarkPubkeys {
			hexpub := npub2hexpub(npub)
			benchmarkRatings[hexpub] = trustRatings[hexpub]
		}

		distrustReportedByUntrusted(benchmarkRatings)
		printRatings(benchmarkRatings)
	}

	// if -i specified, switch to interactive mode
	if interactive {
		for {
			fmt.Printf("Enter a npub to get its rating, or 'x' to exit: ")
			scanner:= bufio.NewScanner(os.Stdin)
			scanner.Scan()
			err := scanner.Err()
			
			if err != nil {
				panic(err)
			}
			if scanner.Text() == "x" {
				return
			}

			interactiveNpub := strings.TrimSpace(scanner.Text())
			interactiveHexpub := npub2hexpub(interactiveNpub)

			interactiveRating := make(map[string]float64)
			interactiveRating[interactiveHexpub] = trustRatings[interactiveHexpub]

			// apply interactive-only functions
			distrustReportedByUntrusted(interactiveRating)

			fmt.Printf("%v  %.10f  %v\n", interactiveNpub, interactiveRating[interactiveHexpub], getDisplayName(interactiveHexpub))
		}
	}

}

