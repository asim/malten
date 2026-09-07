package nature

import "github.com/asim/malten/agent"

func Researcher() agent.Researcher {
	return agent.Researcher{Name: "nature", Objective: "Supply weather estimates and daylight context only for an explicitly identified supported place and time. Read retained data or refresh it when stale. Do not guess where the person is, extrapolate to other places, treat current weather as evidence of historical conditions, or call estimates live observations. An unsupported place or time should remain an uncertainty.", Read: Read, Sources: Context}
}
