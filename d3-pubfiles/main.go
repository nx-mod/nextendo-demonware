// d3-pubfiles — genere les fichiers « publisher » que Diablo III recupere au
// demarrage de sa session en ligne : saison active, evenements communautaires et
// liste noire d'objets.
//
// Ce sont trois fichiers TEXTE que le jeu recoit sous forme de chaine. Les
// formats viennent du consommateur lui-meme (d3hack, season_events.hpp, qui les
// synthetise cote client pour jouer hors ligne) :
//
//	seasons_config.txt    [Season N] + fenetre Start/End
//	config.txt            lignes Cle "valeur", dont tous les CommunityBuff*
//	blacklist_config.txt   sections [GBID] / [SNO]
//
// ATTENTION : generer ces fichiers ne suffit pas encore a les faire arriver dans
// le jeu. Le client les demande par un bdRemoteTask qui passe par la connexion
// lobby (TCP 3074), dont le handshake n'est pas encore resolu. Ce service
// prepare donc le contenu ; la livraison suivra quand le transport existera.
// C'est aussi pour ca qu'il sert les fichiers en HTTP : on peut les relire et
// les valider des maintenant.
//
//	d3-pubfiles [-config pubfiles.json] [-out dir] [-listen :8470]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Config pilote le contenu servi. Un seul fichier a editer pour changer la
// saison ou allumer un evenement.
type Config struct {
	// Saison active. d3hack tournait sur la 37 : on garde la meme par defaut
	// pour que le comportement corresponde a ce qui a deja ete teste.
	Season uint32 `json:"season"`

	// Fenetre de la saison. Le jeu exige « ???, DD MMM YYYY hh:mm:ss GMT » avec
	// un jour sur DEUX chiffres — un jour a un chiffre casse l'analyse.
	SeasonStart string `json:"season_start"`
	SeasonEnd   string `json:"season_end"`

	// Fenetre des buffs communautaires (independante de la saison).
	BuffStart string `json:"buff_start"`
	BuffEnd   string `json:"buff_end"`

	// Evenements communautaires. Chaque entree devient
	// CommunityBuff<Nom> "1"/"0" dans config.txt.
	Events map[string]bool `json:"events"`

	// Multiplicateurs de trouvaille. Le jeu les lit comme des reels.
	LegendaryFind string `json:"legendary_find"`
	GoldFind      string `json:"gold_find"`
	XP            string `json:"xp"`

	// Divers reglages de config.txt qui ne sont pas des evenements.
	HeroPublishFrequencyMinutes string `json:"hero_publish_frequency_minutes"`
	CrossPlatformSaveMigration  bool   `json:"cross_platform_save_migration"`
	SeasonalGlobalLeaderboards  bool   `json:"seasonal_global_leaderboards"`
	Diablo4Advertisement        bool   `json:"diablo4_advertisement"`
	UpdateVersion               string `json:"update_version"`
}

// knownEvents est la liste complete des buffs que le jeu reconnait, dans
// l'ordre ou d3hack les emet. Une cle absente de Config.Events est emise a "0"
// plutot qu'omise : le jeu lit la valeur, et un champ manquant n'est pas la
// meme chose qu'un champ a faux.
var knownEvents = []string{
	"DoubleGoblins",
	"DoubleBountyBags",
	"RoyalGrandeur",
	"LegacyOfNightmares",
	"TriunesWill",
	"Pandemonium",
	"KanaiPowers",
	"TrialsOfTempests",
	"SeasonOnly",
	"ShadowClones",
	"FourthKanaisCubeSlot",
	"EtherealItems",
	"SoulShards",
	"SwarmRifts",
	"SanctifiedItems",
	"DarkAlchemy",
	"ParagonCap",
	"NestingPortals",
	"EasterEggWorld",
	"DoubleRiftKeystones",
	"DoubleBloodShards",
}

func defaultConfig() Config {
	return Config{
		Season:      37,
		SeasonStart: "Sat, 09 Feb 2025 00:00:00 GMT",
		SeasonEnd:   "Tue, 09 Feb 2036 01:00:00 GMT",
		BuffStart:   "Sat, 16 Sep 2023 00:00:00 GMT",
		BuffEnd:     "Wed, 01 Dec 2027 01:00:00 GMT",
		Events: map[string]bool{
			// Rien d'allume par defaut : un serveur doit ressembler a la
			// production tant qu'on ne decide pas le contraire.
		},
		LegendaryFind:               "1.0",
		GoldFind:                    "1.0",
		XP:                          "1.0",
		HeroPublishFrequencyMinutes: "30",
		CrossPlatformSaveMigration:  true,
		SeasonalGlobalLeaderboards:  true,
		Diablo4Advertisement:        false,
		UpdateVersion:               "1",
	}
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// BuildSeasons rend seasons_config.txt. Les deux lignes de commentaire sont
// celles du fichier d'origine : elles documentent la contrainte de format et ne
// coutent rien a garder.
func BuildSeasons(c Config) string {
	var b strings.Builder
	b.WriteString("# Format for dates MUST be: \" ? ? ? , DD MMM YYYY hh : mm:ss UTC\"\n")
	b.WriteString("# The Day of Month(DD) MUST be 2 - digit; use either preceding zero or trailing space\n")
	fmt.Fprintf(&b, "[Season %d]\n", c.Season)
	fmt.Fprintf(&b, "Start \"%s\"\n", c.SeasonStart)
	fmt.Fprintf(&b, "End \"%s\"\n\n", c.SeasonEnd)
	return b.String()
}

// BuildConfig rend config.txt : une ligne Cle "valeur" par reglage.
func BuildConfig(c Config) string {
	var b strings.Builder
	line := func(k, v string) { fmt.Fprintf(&b, "%s \"%s\"\n", k, v) }

	line("HeroPublishFrequencyMinutes", c.HeroPublishFrequencyMinutes)
	line("EnableCrossPlatformSaveMigration", boolStr(c.CrossPlatformSaveMigration))
	line("SeasonalGlobalLeaderboardsEnabled", boolStr(c.SeasonalGlobalLeaderboards))
	line("EnableDiablo4Advertisement", boolStr(c.Diablo4Advertisement))
	line("CommunityBuffStart", c.BuffStart)
	line("CommunityBuffEnd", c.BuffEnd)

	for _, name := range knownEvents {
		line("CommunityBuff"+name, boolStr(c.Events[name]))
	}

	line("CommunityBuffLegendaryFind", c.LegendaryFind)
	line("CommunityBuffGoldFind", c.GoldFind)
	line("CommunityBuffXP", c.XP)
	line("UpdateVersion", c.UpdateVersion)
	return b.String()
}

// BuildBlacklist rend blacklist_config.txt. Les deux sections doivent exister
// meme vides, sinon le jeu n'a rien a analyser.
func BuildBlacklist(_ Config) string {
	return "# Item blacklist served by d3-pubfiles\n[GBID]\n\n[SNO]\n"
}

func loadConfig(path string) (Config, error) {
	c := defaultConfig()
	if path == "" {
		return c, nil
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Premiere execution : on ecrit le fichier par defaut pour qu'il y ait
		// quelque chose a editer, plutot que d'echouer.
		out, _ := json.MarshalIndent(c, "", "  ")
		if werr := os.WriteFile(path, out, 0o644); werr == nil {
			log.Printf("[pubfiles] %s cree avec les valeurs par defaut", path)
		}
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func main() {
	cfgPath := flag.String("config", "pubfiles.json", "fichier de reglages (cree s'il manque)")
	outDir := flag.String("out", "files", "dossier ou ecrire les trois fichiers")
	listen := flag.String("listen", ":8470", "adresse HTTP pour relire les fichiers (vide = pas de serveur)")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("[pubfiles] %v", err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("[pubfiles] %v", err)
	}

	files := map[string]string{
		"seasons_config.txt":   BuildSeasons(cfg),
		"config.txt":           BuildConfig(cfg),
		"blacklist_config.txt": BuildBlacklist(cfg),
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		p := filepath.Join(*outDir, name)
		if err := os.WriteFile(p, []byte(files[name]), 0o644); err != nil {
			log.Printf("[pubfiles] %s: %v", name, err)
			continue
		}
		log.Printf("[pubfiles] %s (%d octets)", p, len(files[name]))
	}

	on := make([]string, 0, len(knownEvents))
	for _, name := range knownEvents {
		if cfg.Events[name] {
			on = append(on, name)
		}
	}
	if len(on) == 0 {
		log.Printf("[pubfiles] saison %d, aucun evenement actif", cfg.Season)
	} else {
		log.Printf("[pubfiles] saison %d, evenements actifs: %s", cfg.Season, strings.Join(on, ", "))
	}

	if *listen == "" {
		return
	}

	mux := http.NewServeMux()
	for _, name := range names {
		body := files[name]
		mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(body))
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, name := range names {
			fmt.Fprintf(w, "/%s\n", name)
		}
	})

	srv := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("[pubfiles] relecture sur http://localhost%s/", *listen)
	log.Fatal(srv.ListenAndServe())
}
