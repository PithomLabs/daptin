package server

import (
	"github.com/artpar/api2go"
	"github.com/artpar/api2go-adapter/gingonic"
	"github.com/daptin/daptin/server/auth"
	"github.com/daptin/daptin/server/resource"
	"github.com/artpar/rclone/fs"
	"github.com/jmoiron/sqlx"
	"github.com/artpar/go.uuid"
	log "github.com/sirupsen/logrus"
	"gopkg.in/gin-gonic/gin.v1"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"github.com/daptin/daptin/server/websockets"
)

var cruds = make(map[string]*resource.DbResource)

func Main(boxRoot, boxStatic http.FileSystem, db *sqlx.DB, wg *sync.WaitGroup, l net.Listener, ch chan struct{}) {
	defer wg.Done()

	//configFile := "daptin_style.json"
	/// Start system initialise

	log.Infof("Load config files")
	initConfig, errs := loadConfigFiles()
	if errs != nil {
		for _, err := range errs {
			log.Errorf("Failed to load config file: %v", err)
		}
	}

	existingTables, _ := GetTablesFromWorld(db)
	//initConfig.Tables = append(initConfig.Tables, existingTables...)
	existingTablesMap := make(map[string]bool)

	allTables := make([]resource.TableInfo, 0)

	for j, existableTable := range existingTables {
		existingTablesMap[existableTable.TableName] = true
		var isBeingModified = false
		var indexBeingModified = -1

		for i, newTable := range initConfig.Tables {
			if newTable.TableName == existableTable.TableName {
				isBeingModified = true
				indexBeingModified = i
				break
			}
		}

		if isBeingModified {
			log.Infof("Table %s is being modified", existableTable.TableName)
			tableBeingModified := initConfig.Tables[indexBeingModified]

			if len(tableBeingModified.Columns) > 0 {

				for _, newColumnDef := range tableBeingModified.Columns {
					columnAlreadyExist := false
					colIndex := -1
					for i, existingColumn := range existableTable.Columns {
						if existingColumn.ColumnName == newColumnDef.ColumnName {
							columnAlreadyExist = true
							colIndex = i
							break
						}
					}
					if columnAlreadyExist {
						//log.Infof("Modifying existing columns[%v][%v] is not supported at present. not sure what would break. and alter query isnt being run currently.", existableTable.TableName, newColumnDef.Name);

						existableTable.Columns[colIndex].DefaultValue = newColumnDef.DefaultValue
						existableTable.Columns[colIndex].ExcludeFromApi = newColumnDef.ExcludeFromApi
						existableTable.Columns[colIndex].IsIndexed = newColumnDef.IsIndexed
						existableTable.Columns[colIndex].IsNullable = newColumnDef.IsNullable
						existableTable.Columns[colIndex].ColumnType = newColumnDef.ColumnType
						existableTable.Columns[colIndex].Options = newColumnDef.Options

					} else {
						existableTable.Columns = append(existableTable.Columns, newColumnDef)
					}

				}

			}
			if len(tableBeingModified.Relations) > 0 {

				existingRelations := existableTable.Relations
				relMap := make(map[string]bool)
				for _, rel := range existingRelations {
					relMap[rel.Hash()] = true
				}

				for _, newRel := range tableBeingModified.Relations {

					_, ok := relMap[newRel.Hash()]
					if !ok {
						existableTable.AddRelation(newRel)
					}
				}
			}
			existingTables[j] = existableTable
		}
		allTables = append(allTables, existableTable)
	}

	for _, newTable := range initConfig.Tables {
		if existingTablesMap[newTable.TableName] {
			continue
		}
		allTables = append(allTables, newTable)
	}

	initConfig.Tables = allTables
	fs.LoadConfig()
	fs.Config.DryRun = false
	fs.Config.LogLevel = 200
	fs.Config.StatsLogLevel = 200

	resource.CheckRelations(&initConfig)
	resource.CheckAuditTables(&initConfig)

	//AddStateMachines(&initConfig, db)

	tx, errb := db.Beginx()
	//_, errb := db.Exec("begin")
	resource.CheckErr(errb, "Failed to begin transaction")

	resource.CheckAllTableStatus(&initConfig, db, tx)
	resource.CreateRelations(&initConfig, tx)
	resource.CreateUniqueConstraints(&initConfig, tx)
	resource.CreateIndexes(&initConfig, tx)
	resource.UpdateWorldTable(&initConfig, tx)
	resource.UpdateWorldColumnTable(&initConfig, tx)
	errc := tx.Commit()
	resource.CheckErr(errc, "Failed to commit transaction")

	resource.UpdateStateMachineDescriptions(&initConfig, db)
	resource.UpdateExchanges(&initConfig, db)
	resource.UpdateStreams(&initConfig, db)
	resource.UpdateMarketplaces(&initConfig, db)
	resource.UpdateStandardData(&initConfig, db)

	err := resource.UpdateActionTable(&initConfig, db)
	resource.CheckErr(err, "Failed to update action table")

	/// end system initialise

	r := gin.Default()
	r.Use(CorsMiddlewareFunc)
	r.StaticFS("/static", boxStatic)

	r.GET("/favicon.ico", func(c *gin.Context) {

		file, err := boxRoot.Open("index.html")
		fileContents, err := ioutil.ReadAll(file)
		_, err = c.Writer.Write(fileContents)
		resource.CheckErr(err, "Failed to write favico")
	})

	configStore, err := resource.NewConfigStore(db)
	jwtSecret, err := configStore.GetConfigValueFor("jwt.secret", "backend")

	if err != nil {
		u, _ := uuid.NewV4()
		newSecret := u.String()
		configStore.SetConfigValueFor("jwt.secret", newSecret, "backend")
		jwtSecret = newSecret
	}

	resource.CheckErr(err, "Failed to get config store")
	err = CheckSystemSecrets(configStore)
	resource.CheckErr(err, "Failed to initialise system secrets")

	r.GET("/config", CreateConfigHandler(configStore))

	authMiddleware := auth.NewAuthMiddlewareBuilder(db)
	auth.InitJwtMiddleware([]byte(jwtSecret))
	r.Use(authMiddleware.AuthCheckMiddleware)

	r.GET("/actions", resource.CreateGuestActionListHandler(&initConfig, cruds))

	api := api2go.NewAPIWithRouting(
		"api",
		api2go.NewStaticResolver("/"),
		gingonic.New(r),
	)

	ms := BuildMiddlewareSet(&initConfig)
	cruds = AddResourcesToApi2Go(api, initConfig.Tables, db, &ms, configStore)

	streamProcessors := GetStreamProcessors(&initConfig, configStore, cruds)
	AddStreamsToApi2Go(api, streamProcessors, db, &ms, configStore)

	hostSwitch := CreateSubSites(&initConfig, db, cruds)

	hostSwitch.handlerMap["api"] = r
	hostSwitch.handlerMap["dashboard"] = r
	go resource.ImportDataFiles(&initConfig, db, cruds)

	authMiddleware.SetUserCrud(cruds["user"])
	authMiddleware.SetUserGroupCrud(cruds["usergroup"])
	authMiddleware.SetUserUserGroupCrud(cruds["user_user_id_has_usergroup_usergroup_id"])

	fsmManager := resource.NewFsmManager(db, cruds)

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	handler := CreateJsModelHandler(&initConfig)
	metaHandler := CreateMetaHandler(&initConfig)
	blueprintHandler := CreateApiBlueprintHandler(&initConfig, cruds)
	modelHandler := CreateReclineModelHandler()
	statsHandler := CreateStatsHandler(&initConfig, cruds)

	r.GET("/jsmodel/:typename", handler)
	r.GET("/stats/:typename", statsHandler)
	r.GET("/meta", metaHandler)
	r.GET("/apispec.raml", blueprintHandler)
	r.GET("/recline_model", modelHandler)
	r.OPTIONS("/jsmodel/:typename", handler)
	r.OPTIONS("/apispec.raml", blueprintHandler)
	r.OPTIONS("/recline_model", modelHandler)

	actionPerformers := GetActionPerformers(&initConfig, configStore)
	initConfig.ActionPerformers = actionPerformers
	//actionPerforMap := make(map[string]resource.ActionPerformerInterface)
	//for _, actionPerformer := range actionPerformers {
	//	actionPerforMap[actionPerformer.Name()] = actionPerformer
	//}
	//initConfig.ActionPerformers = actionPerforMap

	r.POST("/action/:typename/:actionName", resource.CreatePostActionHandler(&initConfig, configStore, cruds, actionPerformers))
	r.GET("/action/:typename/:actionName", resource.CreatePostActionHandler(&initConfig, configStore, cruds, actionPerformers))

	r.POST("/track/start/:stateMachineId", CreateEventStartHandler(fsmManager, cruds, db))
	r.POST("/track/event/:typename/:objectStateId/:eventName", CreateEventHandler(&initConfig, fsmManager, cruds, db))

	r.POST("/site/content/load", CreateSubSiteContentHandler(&initConfig, cruds, db))
	r.POST("/site/content/store", CreateSubSiteSaveContentHandler(&initConfig, cruds, db))

	webSocketConnectionHandler := WebSocketConnectionHandlerImpl{}
	websocketServer := websockets.NewServer("/live", &webSocketConnectionHandler)
	go websocketServer.Listen(r)

	r.NoRoute(func(c *gin.Context) {
		file, err := boxRoot.Open("index.html")
		fileContents, err := ioutil.ReadAll(file)
		_, err = c.Writer.Write(fileContents)
		resource.CheckErr(err, "Failed to write index html")
	})

	resource.InitialiseColumnManager()

	//r.Run(fmt.Sprintf(":%v", *port))
	CleanUpConfigFiles()

	go func() {
		err = http.Serve(l, hostSwitch)
		resource.CheckErr(err, "Failed to listen")
	}()

	select {
	case <-ch:
		return
	default:
	}

}

type WebSocketConnectionHandlerImpl struct {
}

func (wsch *WebSocketConnectionHandlerImpl) MessageFromClient(message websockets.WebSocketPayload, request *http.Request) {

}

func AddStreamsToApi2Go(api *api2go.API, processors []*resource.StreamProcessor, db *sqlx.DB, middlewareSet *resource.MiddlewareSet, configStore *resource.ConfigStore) {

	for _, processor := range processors {

		contract := processor.GetContract()
		model := api2go.NewApi2GoModel(contract.StreamName, contract.Columns, 0, nil)
		api.AddResource(model, processor)

	}

}
func GetStreamProcessors(config *resource.CmsConfig, store *resource.ConfigStore, cruds map[string]*resource.DbResource) []*resource.StreamProcessor {

	allProcessors := make([]*resource.StreamProcessor, 0)

	for _, streamContract := range config.Streams {

		streamProcessor := resource.NewStreamProcessor(streamContract, cruds)
		allProcessors = append(allProcessors, streamProcessor)

	}

	return allProcessors

}
