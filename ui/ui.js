
// Order matters! This way max can be less than min, and things still work.
function clamp(i, min, max) {
	return Math.max(Math.min(i, max), min)
}

var Core = null
var Conn = null

$(document).ready(function() {
	Core = new Vue({
		el: "#Main",
		data: {
			logData: "",
			command: "",

			settings: {
				Addr: "wss://localhost/socket",
				Token: "DEADBEEF",
			},
			settingsShow: false,
			settingsPos: {
				top: (window.innerHeight/2-window.innerHeight/4)+"px", left: (window.innerWidth/3)+"px",
				x: window.innerWidth/3, y: window.innerHeight/2-window.innerHeight/4,
			},
		},
		methods: {
			begindrag: function(id){
				this.dragging = id
			},
			drag: function(evnt){
				// TODO: Works, but rather poorly.
				switch (this.dragging) {
				case 0:
					return
				case 1:
					this.settingsPos.x += evnt.movementX
					this.settingsPos.y += evnt.movementY
					this.settingsPos.left = this.settingsPos.x+"px"
					this.settingsPos.top = this.settingsPos.y+"px"
					return
				}
			},
			dragend: function(){
				this.dragging = 0
			},

			submitCommand: function(){
				Conn.send(`${this.settings.Token}|${this.command}\n`)
				this.command = ""
			},

			saveSettingsBox: function(){
				// Save settings for later.
				window.localStorage["VS.ServerMonitor.Settings"] = JSON.stringify(this.settings)

				this.closeSettingsBox()
			},
			closeSettingsBox: function(){
				this.settingsShow = false
			},
		},
	})

	var savedata = window.localStorage["VS.ServerMonitor.Settings"]
	if (savedata) {
		var settings = JSON.parse(savedata)

		Core.settings.Addr  = settings.Addr  !== undefined ? settings.Addr  : Core.settings.Addr
		Core.settings.Token = settings.Token !== undefined ? settings.Token : Core.settings.Token

		Core.saveSettingsBox()
	}

	Conn = new ReconnectingWebSocket(Core.settings.Addr)
	Conn.onmessage = function(evnt) {
		Core.logData += evnt.data
	}
})
