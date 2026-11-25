package upstream

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
)

func decodeJSON(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, upstreamRejection()
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, upstreamRejection()
	}
	return value, nil
}

func decodeProfileCookie(raw json.RawMessage) error {
	object, err := decodeObject(raw)
	if err != nil {
		return authenticationUnsupported()
	}
	status, ok := exactScalar(object["status"])
	if !ok || status != "success" {
		return authenticationUnsupported()
	}
	cookieValue, ok := object["cookie_id"].(string)
	if !ok {
		return authenticationUnsupported()
	}
	cookieID, ok := boundedScalar(cookieValue, true)
	if !ok || strings.TrimSpace(cookieID) == "" {
		return authenticationUnsupported()
	}
	return nil
}

func decodeProfileHistory(raw json.RawMessage, includeGuest bool) ([]domain.ProfileHistoryRecord, error) {
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	list, ok := value.([]any)
	if !ok {
		return nil, upstreamRejection()
	}
	records := make([]domain.ProfileHistoryRecord, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, upstreamRejection()
		}
		record, ok := decodeProfileHistoryRecord(entry, includeGuest)
		if !ok {
			return nil, upstreamRejection()
		}
		records = append(records, record)
	}
	return records, nil
}

func decodeBlocks(raw json.RawMessage) ([]domain.BlockRecord, error) {
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	list, ok := value.([]any)
	if !ok {
		return nil, upstreamRejection()
	}
	blocks := make([]domain.BlockRecord, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, upstreamRejection()
		}
		id, idOK := boundedScalar(entry["rplb_id"], true)
		name, nameOK := boundedScalar(entry["rplb_tid"], true)
		if !idOK || !nameOK {
			return nil, upstreamRejection()
		}
		blocks = append(blocks, domain.BlockRecord{BlockID: id, TrainerName: name})
	}
	return blocks, nil
}

func decodeProfileHistoryRecord(entry map[string]any, includeGuest bool) (domain.ProfileHistoryRecord, bool) {
	raidID, ok := boundedScalar(entry["rpl_id"], true)
	if !ok {
		return domain.ProfileHistoryRecord{}, false
	}
	displayTime, ok := strictOptionalBoundedScalar(entry, "rpl_insert")
	if !ok {
		return domain.ProfileHistoryRecord{}, false
	}
	pokemonID, ok := boundedScalar(entry["rpl_pokemon"], true)
	if !ok {
		return domain.ProfileHistoryRecord{}, false
	}
	trainerName, ok := strictOptionalBoundedScalar(entry, "rpl_tid")
	if !ok {
		return domain.ProfileHistoryRecord{}, false
	}
	level, ok := profileNonnegativeInt(entry, "rpl_level")
	if !ok {
		return domain.ProfileHistoryRecord{}, false
	}
	country, ok := normalizeCountry(entry["rpl_country_code"], entry["rpl_lang"])
	if !ok {
		return domain.ProfileHistoryRecord{}, false
	}
	rating, ok := profileRating(entry, "evaluatescore")
	if !ok {
		return domain.ProfileHistoryRecord{}, false
	}
	record := domain.ProfileHistoryRecord{RaidID: raidID, DisplayTime: displayTime, PokemonID: pokemonID, TrainerName: trainerName, Level: level, Country: country, Rating: rating}
	if includeGuest {
		guest, ok := decodeProfileGuest(entry)
		if !ok {
			return domain.ProfileHistoryRecord{}, false
		}
		record.Guest = guest
	}
	return record, true
}

func decodeProfileGuest(entry map[string]any) (*domain.ProfileGuest, bool) {
	name, namePresent, ok := optionalBoundedValue(entry, "rplc_tid")
	if !ok {
		return nil, false
	}
	levelValue, levelPresent := entry["rplc_level"]
	countryValue, countryPresent := entry["rplc_country_code"]
	languageValue, languagePresent := entry["rplc_lang"]
	_, ratingPresent := entry["evaluatescore_guest"]
	if !namePresent && !levelPresent && !countryPresent && !languagePresent && !ratingPresent {
		return nil, true
	}
	if !namePresent || strings.TrimSpace(name) == "" {
		return nil, false
	}
	level, ok := optionalNonnegativeInt(entry, "rplc_level", 0)
	if !ok || (levelPresent && levelValue == nil) {
		return nil, false
	}
	country, ok := normalizeCountry(countryValue, languageValue)
	if !ok {
		return nil, false
	}
	rating, ok := profileRating(entry, "evaluatescore_guest")
	if !ok {
		return nil, false
	}
	return &domain.ProfileGuest{TrainerName: name, Level: level, Country: country, Rating: rating}, true
}

func profileNonnegativeInt(object map[string]any, key string) (int, bool) {
	value, present := object[key]
	if !present {
		return 0, true
	}
	if value == nil {
		return 0, false
	}
	return optionalNonnegativeInt(object, key, 0)
}

func profileRating(object map[string]any, key string) (*int, bool) {
	value, present := object[key]
	if !present {
		return nil, true
	}
	if value == nil {
		return nil, false
	}
	return normalizeRating(value, nil)
}

func decodeQueuePoll(raw json.RawMessage) (domain.QueuePoll, error) {
	object, err := decodeObject(raw)
	if err != nil {
		return domain.QueuePoll{}, err
	}
	if err := classifyStatus(object); err != nil {
		return domain.QueuePoll{}, err
	}
	status := optionalInt(object["rpllm_status"])
	if status == nil {
		return domain.QueuePoll{}, upstreamRejection()
	}
	switch *status {
	case 1:
		return domain.QueuePoll{State: domain.QueueWaiting}, nil
	case 2:
		id, ok := boundedScalar(object["rpllm_rpl_id"], true)
		if !ok {
			return domain.QueuePoll{}, upstreamRejection()
		}
		return domain.QueuePoll{State: domain.QueueMatched, RaidID: id}, nil
	default:
		return domain.QueuePoll{}, upstreamRejection()
	}
}

func decodeQueueCheck(raw json.RawMessage) (domain.QueueCheck, error) {
	object, err := decodeObject(raw)
	if err != nil {
		return domain.QueueCheck{}, err
	}
	if err := classifyStatus(object); err != nil {
		return domain.QueueCheck{}, err
	}
	result := domain.QueueCheck{Waiting: []domain.QueueWaitingEntry{}}
	id, present, ok := optionalBoundedValue(object, "id")
	if !ok {
		return domain.QueueCheck{}, upstreamRejection()
	}
	if present && strings.TrimSpace(id) != "" {
		result.Present = true
		result.ID = id
	}
	priority, ok := optionalStrictInt(object, "priority")
	if !ok {
		return domain.QueueCheck{}, upstreamRejection()
	}
	result.Priority = priority
	value, present := object["list"]
	if !present || value == nil {
		return result, nil
	}
	list, ok := value.([]any)
	if !ok {
		return domain.QueueCheck{}, upstreamRejection()
	}
	for _, item := range list {
		switch typed := item.(type) {
		case map[string]any:
			id, _, idOK := optionalBoundedValue(typed, "id")
			rank, _, rankOK := optionalBoundedValue(typed, "rank")
			pokemon, _, pokemonOK := optionalBoundedValue(typed, "pokemon")
			if !idOK || !rankOK || !pokemonOK || (id == "" && rank == "" && pokemon == "") {
				return domain.QueueCheck{}, upstreamRejection()
			}
			result.Waiting = append(result.Waiting, domain.QueueWaitingEntry{ID: id, Rank: rank, PokemonID: pokemon})
		case string, json.Number:
			id, ok := boundedScalar(typed, true)
			if !ok {
				return domain.QueueCheck{}, upstreamRejection()
			}
			result.Waiting = append(result.Waiting, domain.QueueWaitingEntry{ID: id})
		default:
			return domain.QueueCheck{}, upstreamRejection()
		}
	}
	return result, nil
}

func decodeRaidPlayers(raw json.RawMessage) (domain.RaidPlayers, error) {
	object, err := decodeObject(raw)
	if err != nil {
		return domain.RaidPlayers{}, err
	}
	if err := classifyStatus(object); err != nil {
		return domain.RaidPlayers{}, err
	}
	value, present := object["list"]
	list, ok := value.([]any)
	if !present || !ok {
		return domain.RaidPlayers{}, upstreamRejection()
	}
	players := make([]domain.RaidPlayer, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return domain.RaidPlayers{}, upstreamRejection()
		}
		player, ok := decodeRaidPlayer(entry)
		if !ok {
			return domain.RaidPlayers{}, upstreamRejection()
		}
		players = append(players, player)
	}
	count, ok := optionalNonnegativeInt(object, "count", len(players))
	if !ok {
		return domain.RaidPlayers{}, upstreamRejection()
	}
	readyCount, ok := optionalNonnegativeInt(object, "readycount", 0)
	if !ok {
		return domain.RaidPlayers{}, upstreamRejection()
	}
	deleted, ok := optionalZeroOne(object["rpl_delete"])
	if !ok {
		return domain.RaidPlayers{}, upstreamRejection()
	}
	return domain.RaidPlayers{Players: players, Count: count, ReadyCount: readyCount, RaidDeleted: deleted}, nil
}

func decodeRaidPlayer(entry map[string]any) (domain.RaidPlayer, bool) {
	id, ok := optionalStrictScalar(entry, "rplc_id")
	if !ok {
		return domain.RaidPlayer{}, false
	}
	name, ok := optionalStrictScalar(entry, "rplc_tid")
	if !ok {
		return domain.RaidPlayer{}, false
	}
	level, ok := optionalNonnegativeInt(entry, "rplc_level", 0)
	if !ok {
		return domain.RaidPlayer{}, false
	}
	ready, ok := optionalZeroOne(entry["rplc_ready"])
	if !ok {
		return domain.RaidPlayer{}, false
	}
	isMe, ok := optionalZeroOne(entry["my"])
	if !ok {
		return domain.RaidPlayer{}, false
	}
	country, ok := normalizeCountry(entry["rplc_country_code"], entry["rplc_lang"])
	if !ok {
		return domain.RaidPlayer{}, false
	}
	rating, ok := normalizeRating(entry["evaluatescore_guest"], entry["pc_raid_evaluate_score_guest"])
	if !ok {
		return domain.RaidPlayer{}, false
	}
	return domain.RaidPlayer{
		ID: id, TrainerName: name, Level: level, Country: country,
		Rating: rating, Ready: ready, IsMe: isMe,
	}, true
}

func decodeObject(raw json.RawMessage) (map[string]any, error) {
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, upstreamRejection()
	}
	return object, nil
}

func optionalBoundedValue(object map[string]any, key string) (string, bool, bool) {
	value, present := object[key]
	if !present || value == nil {
		return "", false, true
	}
	text, ok := boundedScalar(value, false)
	return text, true, ok
}

func optionalStrictInt(object map[string]any, key string) (*int, bool) {
	value, present := object[key]
	if !present || value == nil {
		return nil, true
	}
	text, ok := exactScalar(value)
	if !ok {
		return nil, false
	}
	if strings.TrimSpace(text) == "" {
		return nil, true
	}
	integer, err := json.Number(strings.TrimSpace(text)).Int64()
	if err != nil || integer < int64(-int(^uint(0)>>1)-1) || integer > int64(^uint(0)>>1) {
		return nil, false
	}
	result := int(integer)
	return &result, true
}

func optionalStrictScalar(object map[string]any, key string) (string, bool) {
	value, present := object[key]
	if !present || value == nil {
		return "", true
	}
	return boundedScalar(value, false)
}

func optionalNonnegativeInt(object map[string]any, key string, fallback int) (int, bool) {
	value, present := object[key]
	if !present || value == nil {
		return fallback, true
	}
	text, ok := exactScalar(value)
	if !ok || strings.TrimSpace(text) == "" {
		return 0, false
	}
	integer, err := json.Number(strings.TrimSpace(text)).Int64()
	if err != nil || integer < 0 || integer > int64(^uint(0)>>1) {
		return 0, false
	}
	return int(integer), true
}

func optionalZeroOne(value any) (bool, bool) {
	if value == nil {
		return false, true
	}
	if boolean, ok := value.(bool); ok {
		return boolean, true
	}
	text, ok := exactScalar(value)
	if !ok {
		return false, false
	}
	switch strings.TrimSpace(text) {
	case "", "0":
		return false, true
	case "1":
		return true, true
	default:
		return false, false
	}
}

func normalizeCountry(countryValue, languageValue any) (string, bool) {
	var country string
	if countryValue != nil {
		var ok bool
		country, ok = exactScalar(countryValue)
		if !ok {
			return "", false
		}
		country = strings.ToLower(strings.TrimSpace(country))
		if country != "" && !safeValue(country, true) {
			return "", false
		}
	}
	var language string
	if languageValue != nil {
		var ok bool
		language, ok = exactScalar(languageValue)
		if !ok {
			return "", false
		}
		language = strings.TrimSpace(language)
	}
	if country != "" {
		return country, true
	}
	switch language {
	case "0":
		return "jp", true
	case "1":
		return "world", true
	default:
		return "", true
	}
}

func normalizeRating(primary, fallback any) (*int, bool) {
	values := []any{primary, fallback}
	texts := make([]string, len(values))
	for index, value := range values {
		if value == nil {
			continue
		}
		text, ok := exactScalar(value)
		if !ok {
			return nil, false
		}
		text = strings.TrimSpace(text)
		texts[index] = text
		if text == "" {
			continue
		}
		if _, err := json.Number(text).Int64(); err != nil {
			return nil, false
		}
	}
	for _, text := range texts {
		if text == "" {
			continue
		}
		integer, _ := json.Number(text).Int64()
		if integer == -1 {
			zero := 0
			return &zero, true
		}
		if integer >= 1 && integer <= 5 {
			rating := int(integer)
			return &rating, true
		}
		return nil, true
	}
	return nil, true
}

func decodeRaids(raw json.RawMessage) ([]domain.Raid, error) {
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	var list []any
	switch typed := value.(type) {
	case []any:
		list = typed
	case map[string]any:
		if err := classifyStatus(typed); err != nil {
			return nil, err
		}
		var ok bool
		list, ok = typed["list"].([]any)
		if !ok {
			return nil, upstreamRejection()
		}
	default:
		return nil, upstreamRejection()
	}
	raids := make([]domain.Raid, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, upstreamRejection()
		}
		raid, ok := decodeRaidEntry(entry)
		if !ok {
			return nil, upstreamRejection()
		}
		raids = append(raids, raid)
	}
	return raids, nil
}

func decodeRaidEntry(entry map[string]any) (domain.Raid, bool) {
	id, ok := boundedScalar(entry["rpl_id"], true)
	if !ok {
		return domain.Raid{}, false
	}
	rank, ok := strictOptionalBoundedScalar(entry, "rpl_rank")
	if !ok {
		return domain.Raid{}, false
	}
	pokemon, ok := strictOptionalBoundedScalar(entry, "rpl_pokemon")
	if !ok {
		return domain.Raid{}, false
	}
	host, ok := strictOptionalBoundedScalar(entry, "rpl_tid")
	if !ok {
		return domain.Raid{}, false
	}
	country, ok := strictOptionalBoundedScalar(entry, "rpl_country_code")
	if !ok {
		return domain.Raid{}, false
	}
	hostLevel, ok := firstStrictNonnegativeInt(entry, "rpl_level", "rpl_host_level", "rpl_tl")
	if !ok {
		return domain.Raid{}, false
	}
	weather, ok := firstStrictNonnegativeInt(entry, "rpl_weather")
	if !ok {
		return domain.Raid{}, false
	}
	maxUsers, ok := firstStrictNonnegativeInt(entry, "rpl_maxuser", "rpl_max_user", "rpl_max_users", "max")
	if !ok {
		return domain.Raid{}, false
	}
	count, ok := firstStrictNonnegativeInt(entry, "cnt", "rpl_count", "count")
	if !ok {
		return domain.Raid{}, false
	}
	readyCount, ok := firstStrictNonnegativeInt(entry, "readycnt", "rpl_ready_count", "readycount")
	if !ok {
		return domain.Raid{}, false
	}
	joinState, ok := optionalStrictInt(entry, "join")
	if !ok {
		return domain.Raid{}, false
	}
	deleted, ok := strictOptionalZeroOne(entry, "rpl_deleted")
	if !ok {
		return domain.Raid{}, false
	}
	isMe, ok := strictOptionalZeroOne(entry, "my")
	if !ok {
		return domain.Raid{}, false
	}
	if value, present := entry["rpl_glevel"]; present && value == nil {
		return domain.Raid{}, false
	}
	minimumGuestLevel, ok := optionalNonnegativeInt(entry, "rpl_glevel", 0)
	if !ok {
		return domain.Raid{}, false
	}
	hostTrainerCode := exactHostTrainerCode(entry["rpl_code"])
	return domain.Raid{
		ID: id, HostTrainerCode: hostTrainerCode, Rank: rank, PokemonID: pokemon, HostName: host, HostLevel: hostLevel, Country: country,
		Weather: weather, MaxUsers: maxUsers, Count: count, ReadyCount: readyCount, JoinState: joinState,
		Deleted: deleted, IsMe: isMe, MinimumGuestLevel: minimumGuestLevel,
	}, true
}

func exactHostTrainerCode(value any) string {
	code, ok := value.(string)
	if !ok || len(code) != 12 {
		return ""
	}
	for _, digit := range []byte(code) {
		if digit < '0' || digit > '9' {
			return ""
		}
	}
	return code
}

func strictOptionalZeroOne(object map[string]any, key string) (bool, bool) {
	value, present := object[key]
	if !present {
		return false, true
	}
	text, ok := exactScalar(value)
	if !ok {
		return false, false
	}
	switch text {
	case "0":
		return false, true
	case "1":
		return true, true
	default:
		return false, false
	}
}

func strictOptionalBoundedScalar(object map[string]any, key string) (string, bool) {
	value, present := object[key]
	if !present || value == nil {
		return "", true
	}
	return boundedScalar(value, false)
}

func firstStrictNonnegativeInt(object map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, present := object[key]
		if !present || value == nil {
			continue
		}
		text, ok := exactScalar(value)
		if !ok || strings.TrimSpace(text) == "" {
			return 0, false
		}
		integer, err := json.Number(strings.TrimSpace(text)).Int64()
		if err != nil || integer < 0 || integer > int64(^uint(0)>>1) {
			return 0, false
		}
		return int(integer), true
	}
	return 0, true
}

func boundedScalar(value any, required bool) (string, bool) {
	text, ok := exactScalar(value)
	if !ok || len(text) > maxQueryValueBytes || (required && strings.TrimSpace(text) == "") || !safeValue(text, false) {
		return "", false
	}
	return text, true
}

func optionalInt(value any) *int {
	if value == nil {
		return nil
	}
	text, ok := exactScalar(value)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	integer, err := json.Number(strings.TrimSpace(text)).Int64()
	if err != nil || integer < int64(-int(^uint(0)>>1)-1) || integer > int64(^uint(0)>>1) {
		return nil
	}
	result := int(integer)
	return &result
}

func decodeRaidBosses(raw json.RawMessage) ([]domain.RaidBoss, error) {
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, upstreamRejection()
	}
	if err := classifyStatus(object); err != nil {
		return nil, err
	}
	list, ok := object["list"].([]any)
	if !ok {
		return nil, upstreamRejection()
	}
	bosses := make([]domain.RaidBoss, 0, len(list))
	type bossKey struct{ rank, pokemon string }
	seen := make(map[bossKey]struct{}, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rank, rankOK := boundedScalar(entry["rank"], true)
		pokemon, pokemonOK := boundedScalar(entry["pokemon"], true)
		if !rankOK || !pokemonOK || pokemon == "egg" {
			continue
		}
		key := bossKey{rank: rank, pokemon: pokemon}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		waiting := nonnegativeInt(entry["offpokemon"])
		bosses = append(bosses, domain.RaidBoss{Rank: rank, PokemonID: pokemon, WaitingCount: waiting})
	}
	return bosses, nil
}

func classifyStatus(object map[string]any) error {
	value, present := object["status"]
	if !present {
		return nil
	}
	status, ok := exactScalar(value)
	if !ok || !strings.EqualFold(strings.TrimSpace(status), "success") {
		return structuredRejection(object)
	}
	return nil
}

func structuredRejection(object map[string]any) error {
	text, _ := exactScalar(object["text"])
	normalized := strings.ToLower(text)
	if strings.Contains(normalized, "session has expired") || strings.Contains(normalized, "session expired") || strings.Contains(normalized, "login required") || strings.Contains(normalized, "please log in") || strings.Contains(normalized, "please login") || restartOnlyMessage(text) {
		return authenticationUnsupported()
	}
	return upstreamRejection()
}

func restartOnlyMessage(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if strings.HasSuffix(normalized, ".") || strings.HasSuffix(normalized, "!") || strings.HasSuffix(normalized, "?") {
		normalized = strings.TrimSpace(normalized[:len(normalized)-1])
	}
	return normalized == "please restart"
}

func exactScalar(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		if _, err := typed.Int64(); err != nil {
			return "", false
		}
		return typed.String(), true
	default:
		return "", false
	}
}

func nonnegativeInt(value any) int {
	text, ok := exactScalar(value)
	if !ok || strings.TrimSpace(text) == "" {
		return 0
	}
	number := json.Number(strings.TrimSpace(text))
	integer, err := number.Int64()
	if err != nil || integer < 0 || integer > int64(^uint(0)>>1) {
		return 0
	}
	return int(integer)
}
