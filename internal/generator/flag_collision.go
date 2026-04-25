package generator

import (
	"fmt"

	"github.com/mvanhorn/cli-printing-press/internal/spec"
)

// dedupeFlagIdentifiers ensures that no two non-positional params on a single
// endpoint share a Go identifier (flag<Camel>) or cobra flag name after
// camelization or kebab-casing, and that no param collides with a reserved
// generator-introduced identifier (pagination's flagAll; async's flagWait,
// flagWaitTimeout, flagWaitInterval).
//
// Conflicting params have Name suffixed with _2, _3, ... until the identifier
// and flag name are both unique. Without this, specs that use date-range
// filter conventions (e.g., Twilio's StartTime, StartTime>, StartTime< all
// camelize to "StartTime") or expose a literal "all" parameter on a paginated
// endpoint (e.g., GitHub notifications colliding with pagination's --all)
// produce duplicate `var flagX` declarations and refuse to compile.
func (g *Generator) dedupeFlagIdentifiers() {
	if g.Spec == nil {
		return
	}
	for resName, res := range g.Spec.Resources {
		for epName, ep := range res.Endpoints {
			idents, flags := reservedFlagNamesForEndpoint(resName, epName, ep, g.AsyncJobs)
			ep.Params = uniquifyParamNames(ep.Params, idents, flags)
			res.Endpoints[epName] = ep
		}
		for subName, sub := range res.SubResources {
			subKey := resName + "." + subName
			for epName, ep := range sub.Endpoints {
				idents, flags := reservedFlagNamesForEndpoint(subKey, epName, ep, g.AsyncJobs)
				ep.Params = uniquifyParamNames(ep.Params, idents, flags)
				sub.Endpoints[epName] = ep
			}
			res.SubResources[subName] = sub
		}
		g.Spec.Resources[resName] = res
	}
}

// reservedFlagNamesForEndpoint returns identifiers and flag names that the
// command templates emit themselves and that user params therefore must not
// shadow.
func reservedFlagNamesForEndpoint(resKey, epName string, ep spec.Endpoint, asyncJobs map[string]AsyncJobInfo) (idents, flags map[string]struct{}) {
	idents = map[string]struct{}{}
	flags = map[string]struct{}{}
	if ep.Pagination != nil {
		idents["flagAll"] = struct{}{}
		flags["all"] = struct{}{}
	}
	if _, isAsync := asyncJobs[resKey+"/"+epName]; isAsync {
		idents["flagWait"] = struct{}{}
		idents["flagWaitTimeout"] = struct{}{}
		idents["flagWaitInterval"] = struct{}{}
		flags["wait"] = struct{}{}
		flags["wait-timeout"] = struct{}{}
		flags["wait-interval"] = struct{}{}
	}
	return idents, flags
}

// uniquifyParamNames returns params with IdentName populated whenever a
// param's Go identifier or cobra flag name would otherwise collide with
// another param earlier in the list or with a reserved generator name. The
// first occurrence of each colliding pattern keeps IdentName empty (templates
// fall back to Name); subsequent ones get IdentName set to Name with _2, _3,
// ... appended. Wire-side serialization always reads from Name and is never
// mutated. Positional params are not flagged and pass through.
func uniquifyParamNames(params []spec.Param, reservedIdents, reservedFlags map[string]struct{}) []spec.Param {
	if len(params) == 0 {
		return params
	}
	usedIdents := map[string]struct{}{}
	usedFlags := map[string]struct{}{}
	for k := range reservedIdents {
		usedIdents[k] = struct{}{}
	}
	for k := range reservedFlags {
		usedFlags[k] = struct{}{}
	}

	out := make([]spec.Param, len(params))
	for i, p := range params {
		if p.Positional {
			out[i] = p
			continue
		}
		ident := "flag" + toCamel(p.Name)
		flag := flagName(p.Name)
		if _, identTaken := usedIdents[ident]; !identTaken {
			if _, flagTaken := usedFlags[flag]; !flagTaken {
				usedIdents[ident] = struct{}{}
				usedFlags[flag] = struct{}{}
				out[i] = p
				continue
			}
		}
		for n := 2; ; n++ {
			candidate := fmt.Sprintf("%s_%d", p.Name, n)
			ident = "flag" + toCamel(candidate)
			flag = flagName(candidate)
			_, identTaken := usedIdents[ident]
			_, flagTaken := usedFlags[flag]
			if !identTaken && !flagTaken {
				p.IdentName = candidate
				usedIdents[ident] = struct{}{}
				usedFlags[flag] = struct{}{}
				out[i] = p
				break
			}
		}
	}
	return out
}

// paramIdent returns the name a Param should use when deriving Go
// identifiers (via camel) or cobra flag names (via flagName). It is
// IdentName when populated by the dedup pass and Name otherwise. The
// resulting string must never be used for wire-side serialization;
// callers writing URL params, JSON keys, or path substitutions read
// Name directly.
func paramIdent(p spec.Param) string {
	if p.IdentName != "" {
		return p.IdentName
	}
	return p.Name
}
